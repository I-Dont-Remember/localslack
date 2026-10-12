package store

import "context"

// Reader is a consistent read snapshot; see Store.Read. It exposes the bulk
// reads that export, diff and state dumps need.
type Reader struct{ tx queryer }

// Team returns the workspace.
func (r *Reader) Team(ctx context.Context) (Team, error) { return teamOf(ctx, r.tx) }

// Users returns all users ordered by id.
func (r *Reader) Users(ctx context.Context) ([]User, error) { return usersOf(ctx, r.tx) }

// Conversations returns all conversations ordered by id.
func (r *Reader) Conversations(ctx context.Context) ([]Conversation, error) {
	return conversationsOf(ctx, r.tx)
}

// MembersOf returns member user ids (sorted) per conversation id.
func (r *Reader) MembersOf(ctx context.Context, ids []string) (map[string][]string, error) {
	return membersOf(ctx, r.tx, ids)
}

// AllMessages returns every message, ordered by channel then ts.
func (r *Reader) AllMessages(ctx context.Context) ([]Message, error) {
	return query(ctx, r.tx, "messages", `SELECT `+msgCols+` FROM messages ORDER BY channel_id, ts`, scanMsg)
}

// Apps returns all apps ordered by id.
func (r *Reader) Apps(ctx context.Context) ([]App, error) { return appsOf(ctx, r.tx) }
