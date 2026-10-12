package store

import (
	"context"
	"fmt"
	"strings"
)

// migrationTable is goose's bookkeeping table; never wiped.
const migrationTable = "goose_db_version"

// Counts is the row count of the workspace tables a reset repopulates.
type Counts struct{ Users, Conversations, Messages int }

// WipeWorkspace deletes every row of every table except the migration history
// and the tables named in keep. Tables are discovered, so tables added by later
// migrations are wiped too. Foreign keys are checked at commit, not per delete,
// so order does not matter; the caller repopulates in the same transaction.
func (t *Tx) WipeWorkspace(ctx context.Context, keep ...string) error {
	if err := t.exec(ctx, "defer foreign keys", `PRAGMA defer_foreign_keys = ON`); err != nil {
		return err
	}
	tables, err := query(ctx, t.tx, "tables",
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`,
		func(r scanner, name *string) error { return r.Scan(name) })
	if err != nil {
		return err
	}
	skip := map[string]bool{migrationTable: true}
	for _, k := range keep {
		skip[k] = true
	}
	for _, name := range tables {
		if skip[name] {
			continue
		}
		if err := t.exec(ctx, "wipe "+name, `DELETE FROM "`+strings.ReplaceAll(name, `"`, `""`)+`"`); err != nil {
			return err
		}
	}
	return nil
}

// Team returns the workspace, seeing this txn's writes.
func (t *Tx) Team(ctx context.Context) (Team, error) { return teamOf(ctx, t.tx) }

// Apps returns all apps ordered by id, seeing this txn's writes.
func (t *Tx) Conversations(ctx context.Context) ([]Conversation, error) {
	return conversationsOf(ctx, t.tx)
}

func (t *Tx) Apps(ctx context.Context) ([]App, error) { return appsOf(ctx, t.tx) }

// MaxTS returns the greatest message ts, or "" if there are none.
func (t *Tx) MaxTS(ctx context.Context) (string, error) { return maxTS(ctx, t.tx) }

// Counts returns the workspace row counts, seeing this txn's writes.
func (t *Tx) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	err := t.tx.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM users), (SELECT count(*) FROM conversations), (SELECT count(*) FROM messages)`).
		Scan(&c.Users, &c.Conversations, &c.Messages)
	if err != nil {
		return Counts{}, fmt.Errorf("count workspace: %w", err)
	}
	return c, nil
}
