package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"localslack/internal/appmanifest"
	"localslack/internal/store"
)

// Errors for the app registry.
var (
	ErrAppNotFound = errors.New("app not found")
	ErrAppExists   = errors.New("app already exists")
	// ErrTokenInUse means a bot_token or app_token already belongs to another app.
	ErrTokenInUse = errors.New("token already belongs to another app")
)

// DefaultBotScopes are granted to an app whose manifest lists none.
func DefaultBotScopes() []string { return []string{"chat:write", "chat:write.public"} }

func appFrom(a store.App) App {
	return App{
		ID: a.ID, Name: a.Name, BotToken: a.BotToken, AppToken: a.AppToken,
		BotUserID: a.BotUserID, BotID: a.BotID, Scopes: a.Scopes, SocketMode: a.SocketMode,
		SigningSecret: a.SigningSecret, VerificationToken: a.VerificationToken,
		RequestURL: a.RequestURL, DeliverTo: a.DeliverTo, BotEvents: a.BotEvents,
		Verified: a.Verified, Manifest: a.Manifest, Created: a.Created,
	}
}

// RefreshApps reloads the token->app cache from the store. Call on startup and
// after any register/delete/revoke/reset.
func (s *Service) RefreshApps(ctx context.Context) error {
	rows, err := s.st.Apps(ctx)
	if err != nil {
		return err
	}
	apps := make([]App, 0, len(rows))
	for _, r := range rows {
		apps = append(apps, appFrom(r))
	}
	s.SetApps(apps...)
	return nil
}

// Apps returns the registered apps, ordered by id, from the cache.
func (s *Service) Apps() []App { return s.apps.all() }

// App returns one registered app.
func (s *Service) App(ctx context.Context, id string) (App, error) {
	row, err := s.st.App(ctx, id)
	if err != nil {
		return App{}, notFoundAs(err, ErrAppNotFound)
	}
	return appFrom(row), nil
}

// RegisterInput describes an app to register. Empty credential fields get
// deterministic defaults derived from the app id.
type RegisterInput struct {
	Manifest                          appmanifest.Parsed
	AppID                             string // default: derived from the manifest name
	SigningSecret, BotToken, AppToken string
	// DeliverTo overrides where events go; empty keeps a stored value.
	DeliverTo string
	// Upsert updates an existing app instead of failing with ErrAppExists.
	Upsert bool
}

// RegisterApp adds an app: its bot user (with bot_id, joined to every public
// channel), credentials and subscriptions. With Upsert an existing app keeps
// its id, bot user, credentials and stored deliver_to, and takes the manifest's
// other fields. The token cache is refreshed.
func (s *Service) RegisterApp(ctx context.Context, in RegisterInput) (App, error) {
	m := in.Manifest.Manifest
	id := in.AppID
	if id == "" {
		id = derivedID('A', "app/"+strings.ToLower(m.DisplayInformation.Name))
	}
	existing, err := s.st.App(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return App{}, err
	}
	if found && !in.Upsert {
		return App{}, fmt.Errorf("%w: %s", ErrAppExists, id)
	}
	row := newAppRow(id, in, existing, found, s.now().Unix())
	if err := s.checkTokensFree(id, row); err != nil {
		return App{}, err
	}
	team, err := s.st.Team(ctx)
	if err != nil {
		return App{}, err
	}
	err = s.st.Write(ctx, func(tx *store.Tx) error {
		if err := ensureBotUser(ctx, tx, team.ID, row, m.Features.BotUser.DisplayName); err != nil {
			return err
		}
		if found {
			return tx.UpdateApp(ctx, row)
		}
		return tx.InsertApp(ctx, row)
	})
	if err != nil {
		return App{}, err
	}
	s.tombs.remove(row.BotToken, row.AppToken) // a re-registered app owns its tokens again
	if err := s.RefreshApps(ctx); err != nil {
		return App{}, err
	}
	return s.App(ctx, id)
}

// checkTokensFree fails with ErrTokenInUse when row's tokens belong to another app.
func (s *Service) checkTokensFree(id string, row store.App) error {
	for _, other := range s.apps.all() {
		if other.ID == id {
			continue
		}
		switch {
		case row.BotToken != "" && (row.BotToken == other.BotToken || row.BotToken == other.AppToken),
			row.AppToken != "" && (row.AppToken == other.AppToken || row.AppToken == other.BotToken):
			return fmt.Errorf("%w: app %s", ErrTokenInUse, other.ID)
		}
	}
	return nil
}

func newAppRow(id string, in RegisterInput, old store.App, found bool, now int64) store.App {
	m := in.Manifest.Manifest
	scopes := m.OAuthConfig.Scopes.Bot
	if len(scopes) == 0 {
		scopes = DefaultBotScopes()
	}
	row := store.App{
		ID: id, Name: m.DisplayInformation.Name, Manifest: in.Manifest.Raw,
		RequestURL: m.Settings.EventSubscriptions.RequestURL, BotEvents: m.Settings.EventSubscriptions.BotEvents,
		Scopes: scopes, SocketMode: m.Settings.SocketModeEnabled,
		SigningSecret:     pick(in.SigningSecret, old.SigningSecret, hexOf("signing/"+id, 32)),
		VerificationToken: pick("", old.VerificationToken, hexOf("verification/"+id, 24)),
		BotToken:          pick(in.BotToken, old.BotToken, "xo"+"xb-"+id+"-"+hexOf("bot/"+id, 8)),
		AppToken:          pick(in.AppToken, old.AppToken, "xa"+"pp-1-"+id+"-"+hexOf("app/"+id, 8)),
		BotUserID:         pick("", old.BotUserID, derivedID('U', "user/"+id)),
		BotID:             pick("", old.BotID, derivedID('B', "bot/"+id)),
		DeliverTo:         pick(in.DeliverTo, old.DeliverTo, ""),
		Verified:          old.Verified, Created: now,
	}
	if found {
		row.Created = old.Created
	}
	return row
}

// pick returns the first non-empty value: explicit, then stored, then derived.
func pick(explicit, stored, derived string) string {
	for _, v := range []string{explicit, stored, derived} {
		if v != "" {
			return v
		}
	}
	return ""
}

// JoinBotToPublicChannels adds botUserID to every public, unarchived channel
// in the tx. Reusable by anything that repopulates the workspace while apps
// persist (partial reset today; scenario populate / snapshot restore later):
// call it inside the same write tx, after the new channels exist. Idempotent.
func JoinBotToPublicChannels(ctx context.Context, tx *store.Tx, botUserID string) error {
	convs, err := tx.Conversations(ctx)
	if err != nil {
		return err
	}
	for _, c := range convs {
		if c.Kind == KindChannel && !c.IsPrivate && !c.IsArchived {
			if err := tx.AddMember(ctx, c.ID, botUserID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureBotUser creates the app's bot user and joins it to public channels if it
// does not exist yet; an existing bot user is left alone.
func ensureBotUser(ctx context.Context, tx *store.Tx, teamID string, app store.App, display string) error {
	_, err := tx.User(ctx, app.BotUserID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if display == "" {
		display = app.Name
	}
	bot := store.User{ID: app.BotUserID, TeamID: teamID, Name: botUsername(display), RealName: display, IsBot: true, BotID: app.BotID}
	if err := tx.InsertUser(ctx, bot); err != nil {
		return err
	}
	return JoinBotToPublicChannels(ctx, tx, bot.ID)
}

// DeleteApp removes an app and revokes its tokens: they answer invalid_auth,
// even in permissive mode, until reset --all or restart. Its bot user and
// messages stay.
func (s *Service) DeleteApp(ctx context.Context, id string) error {
	var row store.App
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		var err error
		if row, err = tx.App(ctx, id); err != nil {
			return err
		}
		return tx.DeleteApp(ctx, id)
	})
	if err != nil {
		return notFoundAs(err, ErrAppNotFound)
	}
	s.tombs.add(row.BotToken, row.AppToken)
	return s.RefreshApps(ctx)
}

// SetAppDeliverTo stores where the app's events go ("" clears the override).
func (s *Service) SetAppDeliverTo(ctx context.Context, id, deliverTo string) (App, error) {
	if err := s.updateApp(ctx, id, func(a *store.App) { a.DeliverTo = deliverTo }); err != nil {
		return App{}, err
	}
	return s.App(ctx, id)
}

// SetAppVerified records the outcome of a url_verification handshake.
func (s *Service) SetAppVerified(ctx context.Context, id string, verified bool) error {
	return s.updateApp(ctx, id, func(a *store.App) { a.Verified = verified })
}

func (s *Service) updateApp(ctx context.Context, id string, mutate func(*store.App)) error {
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		row, err := tx.App(ctx, id)
		if err != nil {
			return err
		}
		mutate(&row)
		return tx.UpdateApp(ctx, row)
	})
	if err != nil {
		return notFoundAs(err, ErrAppNotFound)
	}
	return s.RefreshApps(ctx)
}

const idAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// derivedID is a Slack-style id (prefix + 10 chars) determined by seed.
func derivedID(prefix byte, seed string) string {
	sum := sha256.Sum256([]byte(seed))
	b := make([]byte, 11)
	b[0] = prefix
	for i := 1; i < len(b); i++ {
		b[i] = idAlphabet[int(sum[i])%len(idAlphabet)]
	}
	return string(b)
}

// hexOf is n hex characters determined by seed.
func hexOf(seed string, n int) string {
	sum := sha256.Sum256([]byte("localslack/" + seed))
	return hex.EncodeToString(sum[:])[:n]
}

var nonUsername = regexp.MustCompile(`[^a-z0-9._-]+`)

// botUsername makes a Slack-style handle from a display name.
func botUsername(display string) string {
	name := strings.Trim(nonUsername.ReplaceAllString(strings.ToLower(display), "-"), "-")
	if name == "" {
		return "app"
	}
	return name
}
