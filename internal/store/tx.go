package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Tx is a write transaction handed to Store.Write callbacks.
type Tx struct{ tx *sql.Tx }

func (t *Tx) exec(ctx context.Context, what, q string, args ...any) error {
	if _, err := t.tx.ExecContext(ctx, q, args...); err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && (se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY) {
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
		`INSERT INTO users (id, team_id, name, real_name, is_bot, bot_id) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.TeamID, u.Name, u.RealName, u.IsBot, u.BotID)
}

// InsertConversation adds a conversation; a duplicate channel name yields ErrDuplicate.
func (t *Tx) InsertConversation(ctx context.Context, c Conversation) error {
	return t.exec(ctx, "insert conversation",
		`INSERT INTO conversations (id, team_id, name, kind, creator, created, is_archived, is_private) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TeamID, c.Name, c.Kind, c.Creator, c.Created, c.IsArchived, c.IsPrivate)
}

// AddMember adds a user to a conversation.
func (t *Tx) AddMember(ctx context.Context, conversationID, userID string) error {
	return t.exec(ctx, "add member", `INSERT INTO members (conversation_id, user_id) VALUES (?, ?)`, conversationID, userID)
}

// User returns a user by id, seeing this txn's writes.
func (t *Tx) User(ctx context.Context, id string) (User, error) { return userByID(ctx, t.tx, id) }

// Conversation returns a conversation by id, seeing this txn's writes.
func (t *Tx) Conversation(ctx context.Context, id string) (Conversation, error) {
	return conversationByID(ctx, t.tx, id)
}

// ConversationWithMembers returns the id of the conversation of the given kind whose
// member set is exactly userIDs, or "" if none exists.
func (t *Tx) ConversationWithMembers(ctx context.Context, kind string, userIDs []string) (string, error) {
	set := slices.Clone(userIDs)
	slices.Sort(set)
	set = slices.Compact(set)
	args := []any{kind, len(set)}
	for _, id := range set {
		args = append(args, id)
	}
	args = append(args, len(set))
	var id string
	err := t.tx.QueryRowContext(ctx, `
		SELECT c.id FROM conversations c
		WHERE c.kind = ?
		  AND (SELECT count(*) FROM members m WHERE m.conversation_id = c.id) = ?
		  AND (SELECT count(*) FROM members m WHERE m.conversation_id = c.id AND m.user_id IN (`+placeholders(len(set))+`)) = ?
		ORDER BY c.id LIMIT 1`, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find conversation by members: %w", err)
	}
	return id, nil
}

// SetArchived flips a conversation's archived flag; ErrNotFound if it does not exist.
func (t *Tx) SetArchived(ctx context.Context, id string, archived bool) error {
	res, err := t.tx.ExecContext(ctx, `UPDATE conversations SET is_archived = ? WHERE id = ?`, archived, id)
	if err != nil {
		return fmt.Errorf("set archived: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("set archived %s: %w", id, ErrNotFound)
	}
	return nil
}
