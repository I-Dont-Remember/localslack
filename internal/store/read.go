package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("store: not found")

// queryer is satisfied by *sql.DB and *sql.Tx, so reads work on either pool or inside a write txn.
type queryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

const (
	userCols = `id, team_id, name, real_name, is_bot, bot_id`
	convCols = `id, team_id, name, kind, creator, created, is_archived, is_private`
)

func scanUser(r scanner, u *User) error {
	return r.Scan(&u.ID, &u.TeamID, &u.Name, &u.RealName, &u.IsBot, &u.BotID)
}

func scanConv(r scanner, c *Conversation) error {
	return r.Scan(&c.ID, &c.TeamID, &c.Name, &c.Kind, &c.Creator, &c.Created, &c.IsArchived, &c.IsPrivate)
}

// IsEmpty reports whether the database has no teams (i.e. is unseeded).
func (s *Store) IsEmpty(ctx context.Context) (bool, error) {
	var n int
	if err := s.reader.QueryRowContext(ctx, `SELECT count(*) FROM teams`).Scan(&n); err != nil {
		return false, fmt.Errorf("count teams: %w", err)
	}
	return n == 0, nil
}

// MaxTS returns the greatest message ts, or "" if there are no messages.
func (s *Store) MaxTS(ctx context.Context) (string, error) {
	var ts sql.NullString
	if err := s.reader.QueryRowContext(ctx, `SELECT max(ts) FROM messages`).Scan(&ts); err != nil {
		return "", fmt.Errorf("max ts: %w", err)
	}
	return ts.String, nil
}

// Team returns the workspace (LocalSlack hosts exactly one).
func (s *Store) Team(ctx context.Context) (Team, error) { return teamOf(ctx, s.reader) }

func teamOf(ctx context.Context, q queryer) (Team, error) {
	return getOne(ctx, q, "team", `SELECT id, name, domain FROM teams ORDER BY id LIMIT 1`,
		func(r scanner, t *Team) error { return r.Scan(&t.ID, &t.Name, &t.Domain) })
}

// User returns a user by id.
func (s *Store) User(ctx context.Context, id string) (User, error) {
	return userByID(ctx, s.reader, id)
}

// BotUser returns the workspace bot user.
func (s *Store) BotUser(ctx context.Context) (User, error) {
	return getOne(ctx, s.reader, "bot user", `SELECT `+userCols+` FROM users WHERE is_bot = 1 ORDER BY id LIMIT 1`, scanUser)
}

// Users returns all users ordered by id.
func (s *Store) Users(ctx context.Context) ([]User, error) { return usersOf(ctx, s.reader) }

func usersOf(ctx context.Context, q queryer) ([]User, error) {
	return query(ctx, q, "users", `SELECT `+userCols+` FROM users ORDER BY id`, scanUser)
}

// Conversation returns a conversation by id.
func (s *Store) Conversation(ctx context.Context, id string) (Conversation, error) {
	return conversationByID(ctx, s.reader, id)
}

// Conversations returns all conversations ordered by id.
func (s *Store) Conversations(ctx context.Context) ([]Conversation, error) {
	return conversationsOf(ctx, s.reader)
}

func conversationsOf(ctx context.Context, q queryer) ([]Conversation, error) {
	return query(ctx, q, "conversations", `SELECT `+convCols+` FROM conversations ORDER BY id`, scanConv)
}

func userByID(ctx context.Context, q queryer, id string) (User, error) {
	return getOne(ctx, q, "user", `SELECT `+userCols+` FROM users WHERE id = ?`, scanUser, id)
}

func conversationByID(ctx context.Context, q queryer, id string) (Conversation, error) {
	return getOne(ctx, q, "conversation", `SELECT `+convCols+` FROM conversations WHERE id = ?`, scanConv, id)
}

func getOne[T any](ctx context.Context, q queryer, what, sqlStr string, scan func(scanner, *T) error, args ...any) (T, error) {
	var v T
	if err := scan(q.QueryRowContext(ctx, sqlStr, args...), &v); err != nil {
		var zero T
		if errors.Is(err, sql.ErrNoRows) {
			return zero, fmt.Errorf("%s: %w", what, ErrNotFound)
		}
		return zero, fmt.Errorf("get %s: %w", what, err)
	}
	return v, nil
}

func query[T any](ctx context.Context, q queryer, what, sqlStr string, scan func(scanner, *T) error, args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var v T
		if err := scan(rows, &v); err != nil {
			return nil, fmt.Errorf("scan %s: %w", what, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", what, err)
	}
	return out, nil
}
