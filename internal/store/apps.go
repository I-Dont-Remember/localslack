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
}

const appCols = `id, name, manifest, signing_secret, bot_token, app_token, verification_token, request_url,
	bot_events, scopes, socket_mode, verified, bot_user_id, bot_id, created`

func scanApp(r scanner, a *App) error {
	var manifest, events, scopes string
	err := r.Scan(&a.ID, &a.Name, &manifest, &a.SigningSecret, &a.BotToken, &a.AppToken, &a.VerificationToken,
		&a.RequestURL, &events, &scopes, &a.SocketMode, &a.Verified, &a.BotUserID, &a.BotID, &a.Created)
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
		`INSERT INTO apps (`+appCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, manifest, a.SigningSecret, a.BotToken, a.AppToken, a.VerificationToken, a.RequestURL,
		string(events), string(scopes), a.SocketMode, a.Verified, a.BotUserID, a.BotID, a.Created)
}

// DeleteApp removes an app; ErrNotFound if it does not exist.
func (t *Tx) DeleteApp(ctx context.Context, id string) error {
	res, err := t.tx.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("delete app %s: %w", id, ErrNotFound)
	}
	return nil
}

// App returns an app by id.
func (s *Store) App(ctx context.Context, id string) (App, error) {
	return getOne(ctx, s.reader, "app", `SELECT `+appCols+` FROM apps WHERE id = ?`, scanApp, id)
}

// Apps returns all apps ordered by id.
func (s *Store) Apps(ctx context.Context) ([]App, error) {
	return query(ctx, s.reader, "apps", `SELECT `+appCols+` FROM apps ORDER BY id`, scanApp)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
