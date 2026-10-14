package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Bulk inserts many rows through statements prepared once per transaction,
// for populate/restore. Close it before the transaction ends.
type Bulk struct {
	user, conv, member, msg, reaction *sql.Stmt
}

// Bulk prepares the bulk insert statements on this transaction.
func (t *Tx) Bulk(ctx context.Context) (*Bulk, error) {
	b := &Bulk{}
	prep := func(dst **sql.Stmt, q string) error {
		st, err := t.tx.PrepareContext(ctx, q)
		if err != nil {
			return fmt.Errorf("prepare bulk insert: %w", err)
		}
		*dst = st
		return nil
	}
	steps := []error{
		prep(&b.user, `INSERT INTO users (id, team_id, name, real_name, is_bot, bot_id) VALUES (?, ?, ?, ?, ?, ?)`),
		prep(&b.conv, `INSERT INTO conversations (id, team_id, name, kind, creator, created, is_archived, is_private) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		prep(&b.member, `INSERT INTO members (conversation_id, user_id) VALUES (?, ?)`),
		prep(&b.msg, `INSERT INTO messages (channel_id, ts, user_id, text, thread_ts, reply_count, latest_reply, data) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		prep(&b.reaction, `INSERT INTO reactions (`+reactionCols+`) VALUES (?, ?, ?, ?, ?)`),
	}
	for _, err := range steps {
		if err != nil {
			_ = b.Close()
			return nil, err
		}
	}
	return b, nil
}

// Close releases the prepared statements.
func (b *Bulk) Close() error {
	var errs []error
	for _, st := range []*sql.Stmt{b.user, b.conv, b.member, b.msg, b.reaction} {
		if st != nil {
			errs = append(errs, st.Close())
		}
	}
	return errors.Join(errs...)
}

// User adds a user.
func (b *Bulk) User(ctx context.Context, u User) error {
	_, err := b.user.ExecContext(ctx, u.ID, u.TeamID, u.Name, u.RealName, u.IsBot, u.BotID)
	return execErr("insert user", err)
}

// Conversation adds a conversation.
func (b *Bulk) Conversation(ctx context.Context, c Conversation) error {
	_, err := b.conv.ExecContext(ctx, c.ID, c.TeamID, c.Name, c.Kind, c.Creator, c.Created, c.IsArchived, c.IsPrivate)
	return execErr("insert conversation", err)
}

// Member adds a user to a conversation.
func (b *Bulk) Member(ctx context.Context, conversationID, userID string) error {
	_, err := b.member.ExecContext(ctx, conversationID, userID)
	return execErr("add member", err)
}

// Message adds a message.
func (b *Bulk) Message(ctx context.Context, m Message) error {
	data, err := encodeData(m)
	if err != nil {
		return err
	}
	_, err = b.msg.ExecContext(ctx, m.ChannelID, m.TS, m.UserID, m.Text, m.ThreadTS, m.ReplyCount, m.LatestReply, data)
	return execErr("insert message", err)
}

// Reaction adds a reaction.
func (b *Bulk) Reaction(ctx context.Context, r Reaction) error {
	_, err := b.reaction.ExecContext(ctx, r.ChannelID, r.TS, r.Name, r.UserID, r.Created)
	return execErr("insert reaction", err)
}

// IsEmpty reports whether there is no team yet (an unseeded database), seeing this txn's writes.
func (t *Tx) IsEmpty(ctx context.Context) (bool, error) {
	var n int
	if err := t.tx.QueryRowContext(ctx, `SELECT count(*) FROM teams`).Scan(&n); err != nil {
		return false, fmt.Errorf("count teams: %w", err)
	}
	return n == 0, nil
}

// RenameTeam sets the workspace's name and domain (the team id never changes).
func (t *Tx) RenameTeam(ctx context.Context, name, domain string) error {
	return t.exec(ctx, "rename team", `UPDATE teams SET name = ?, domain = ?`, name, domain)
}
