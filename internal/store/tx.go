package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Tx is a write transaction handed to Store.Write callbacks.
type Tx struct{ tx *sql.Tx }

func (t *Tx) exec(ctx context.Context, what, q string, args ...any) error {
	if _, err := t.tx.ExecContext(ctx, q, args...); err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("%s: %w", what, ErrDuplicate)
		}
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// InsertTeam adds a team.
func (t *Tx) InsertTeam(ctx context.Context, m Team) error {
	return t.exec(ctx, "insert team", `INSERT INTO teams (id, name, domain) VALUES (?, ?, ?)`, m.ID, m.Name, m.Domain)
}

// InsertUser adds a user.
func (t *Tx) InsertUser(ctx context.Context, u User) error {
	return t.exec(ctx, "insert user",
		`INSERT INTO users (id, team_id, name, real_name, is_bot) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.TeamID, u.Name, u.RealName, u.IsBot)
}

// InsertConversation adds a conversation; a duplicate channel name yields ErrDuplicate.
func (t *Tx) InsertConversation(ctx context.Context, c Conversation) error {
	return t.exec(ctx, "insert conversation",
		`INSERT INTO conversations (id, team_id, name, kind, creator, created, is_archived) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TeamID, c.Name, c.Kind, c.Creator, c.Created, c.IsArchived)
}

// AddMember adds a user to a conversation.
func (t *Tx) AddMember(ctx context.Context, conversationID, userID string) error {
	return t.exec(ctx, "add member", `INSERT INTO members (conversation_id, user_id) VALUES (?, ?)`, conversationID, userID)
}

// InsertMessage adds a message.
func (t *Tx) InsertMessage(ctx context.Context, m Message) error {
	return t.exec(ctx, "insert message",
		`INSERT INTO messages (channel_id, ts, user_id, text, thread_ts, reply_count, latest_reply) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ChannelID, m.TS, m.UserID, m.Text, m.ThreadTS, m.ReplyCount, m.LatestReply)
}
