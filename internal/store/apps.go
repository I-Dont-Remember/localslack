package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// App is a registered Slack app row.
type App struct {
	ID, Name                          string
	Manifest                          json.RawMessage
	SigningSecret, BotToken, AppToken string
	VerificationToken, RequestURL     string
	BotEvents, Scopes                 []string
	SocketMode, Verified              bool
	BotUserID, BotID                  string
	Created                           int64
	// DeliverTo overrides where events go (host-only or full URL); "" = unset.
	DeliverTo string
}

const appCols = `id, name, manifest, signing_secret, bot_token, app_token, verification_token, request_url,
	bot_events, scopes, socket_mode, verified, bot_user_id, bot_id, created, deliver_to`

func scanApp(r scanner, a *App) error {
	var manifest, events, scopes string
	err := r.Scan(&a.ID, &a.Name, &manifest, &a.SigningSecret, &a.BotToken, &a.AppToken, &a.VerificationToken,
		&a.RequestURL, &events, &scopes, &a.SocketMode, &a.Verified, &a.BotUserID, &a.BotID, &a.Created, &a.DeliverTo)
	if err != nil {
		return err
	}
	a.Manifest = json.RawMessage(manifest)
	if err := json.Unmarshal([]byte(events), &a.BotEvents); err != nil {
		return fmt.Errorf("decode bot_events: %w", err)
	}
	if err := json.Unmarshal([]byte(scopes), &a.Scopes); err != nil {
		return fmt.Errorf("decode scopes: %w", err)
	}
	return nil
}

// InsertApp adds an app; a duplicate id yields ErrDuplicate.
func (t *Tx) InsertApp(ctx context.Context, a App) error {
	events, err := json.Marshal(nonNil(a.BotEvents))
	if err != nil {
		return fmt.Errorf("encode bot_events: %w", err)
	}
	scopes, err := json.Marshal(nonNil(a.Scopes))
	if err != nil {
		return fmt.Errorf("encode scopes: %w", err)
	}
	manifest := string(a.Manifest)
	if manifest == "" {
		manifest = "{}"
	}
	return t.exec(ctx, "insert app",
		`INSERT INTO apps (`+appCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, manifest, a.SigningSecret, a.BotToken, a.AppToken, a.VerificationToken, a.RequestURL,
		string(events), string(scopes), a.SocketMode, a.Verified, a.BotUserID, a.BotID, a.Created, a.DeliverTo)
}

// UpdateApp rewrites every column of an app except id and created;
// ErrNotFound if it does not exist.
func (t *Tx) UpdateApp(ctx context.Context, a App) error {
	events, err := json.Marshal(nonNil(a.BotEvents))
	if err != nil {
		return fmt.Errorf("encode bot_events: %w", err)
	}
	scopes, err := json.Marshal(nonNil(a.Scopes))
	if err != nil {
		return fmt.Errorf("encode scopes: %w", err)
	}
	manifest := string(a.Manifest)
	if manifest == "" {
		manifest = "{}"
	}
	return t.execOne(ctx, "update app", "update app "+a.ID,
		`UPDATE apps SET name = ?, manifest = ?, signing_secret = ?, bot_token = ?, app_token = ?,
		 verification_token = ?, request_url = ?, bot_events = ?, scopes = ?, socket_mode = ?, verified = ?,
		 bot_user_id = ?, bot_id = ?, deliver_to = ? WHERE id = ?`,
		a.Name, manifest, a.SigningSecret, a.BotToken, a.AppToken, a.VerificationToken, a.RequestURL,
		string(events), string(scopes), a.SocketMode, a.Verified, a.BotUserID, a.BotID, a.DeliverTo, a.ID)
}

// DeleteApp removes an app; ErrNotFound if it does not exist.
func (t *Tx) DeleteApp(ctx context.Context, id string) error {
	return t.execOne(ctx, "delete app", "delete app "+id, `DELETE FROM apps WHERE id = ?`, id)
}

// App returns an app by id, seeing this txn's writes.
func (t *Tx) App(ctx context.Context, id string) (App, error) {
	return getOne(ctx, t.tx, "app", `SELECT `+appCols+` FROM apps WHERE id = ?`, scanApp, id)
}

// App returns an app by id.
func (s *Store) App(ctx context.Context, id string) (App, error) {
	return getOne(ctx, s.reader, "app", `SELECT `+appCols+` FROM apps WHERE id = ?`, scanApp, id)
}

// Apps returns all apps ordered by id.
func (s *Store) Apps(ctx context.Context) ([]App, error) { return appsOf(ctx, s.reader) }

func appsOf(ctx context.Context, q queryer) ([]App, error) {
	return query(ctx, q, "apps", `SELECT `+appCols+` FROM apps ORDER BY id`, scanApp)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
