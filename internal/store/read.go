package store

import (
	"context"
	"database/sql"
	"fmt"
)

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

// Users returns all users ordered by id.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	return query(ctx, s.reader, "users",
		`SELECT id, team_id, name, real_name, is_bot FROM users ORDER BY id`,
		func(r *sql.Rows, u *User) error { return r.Scan(&u.ID, &u.TeamID, &u.Name, &u.RealName, &u.IsBot) })
}

// Conversations returns all conversations ordered by id.
func (s *Store) Conversations(ctx context.Context) ([]Conversation, error) {
	return query(ctx, s.reader, "conversations",
		`SELECT id, team_id, name, kind, creator, created, is_archived FROM conversations ORDER BY id`,
		func(r *sql.Rows, c *Conversation) error {
			return r.Scan(&c.ID, &c.TeamID, &c.Name, &c.Kind, &c.Creator, &c.Created, &c.IsArchived)
		})
}

// Messages returns a channel's messages ordered oldest first.
func (s *Store) Messages(ctx context.Context, channelID string) ([]Message, error) {
	return query(ctx, s.reader, "messages",
		`SELECT channel_id, ts, user_id, text, thread_ts, reply_count, latest_reply
		 FROM messages WHERE channel_id = ? ORDER BY ts`,
		func(r *sql.Rows, m *Message) error {
			return r.Scan(&m.ChannelID, &m.TS, &m.UserID, &m.Text, &m.ThreadTS, &m.ReplyCount, &m.LatestReply)
		}, channelID)
}

func query[T any](ctx context.Context, db *sql.DB, what, q string, scan func(*sql.Rows, *T) error, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, q, args...)
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
