package store

import (
	"context"
	"strings"
)

// ConvQuery selects conversations. Zero values mean "no filter" (Limit 0 = unlimited).
type ConvQuery struct {
	Kinds           []string
	ExcludeArchived bool
	// MemberID restricts non-channel kinds (im, mpim) to conversations this user is in.
	MemberID string
	// AfterID is an exclusive keyset cursor on conversation id.
	AfterID string
	Limit   int
}

// ConversationsPage returns conversations ordered by id.
func (s *Store) ConversationsPage(ctx context.Context, cq ConvQuery) ([]Conversation, error) {
	where := []string{"1=1"}
	var args []any
	if len(cq.Kinds) > 0 {
		where = append(where, "kind IN ("+placeholders(len(cq.Kinds))+")")
		for _, k := range cq.Kinds {
			args = append(args, k)
		}
	}
	if cq.ExcludeArchived {
		where = append(where, "is_archived = 0")
	}
	if cq.MemberID != "" {
		where = append(where, `(kind = 'channel' OR EXISTS (SELECT 1 FROM members m WHERE m.conversation_id = conversations.id AND m.user_id = ?))`)
		args = append(args, cq.MemberID)
	}
	if cq.AfterID != "" {
		where = append(where, "id > ?")
		args = append(args, cq.AfterID)
	}
	q := `SELECT ` + convCols + ` FROM conversations WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id`
	q, args = withLimit(q, args, cq.Limit)
	return query(ctx, s.reader, "conversations page", q, scanConv, args...)
}

// HistoryQuery selects top-level messages. Zero values mean "no bound".
// Before is an exclusive keyset cursor; Oldest/Latest are Slack range bounds
// (exclusive unless Inclusive). Limit 0 means unlimited.
type HistoryQuery struct {
	Before, Oldest, Latest string
	Inclusive              bool
	Limit                  int
}

// HistoryPage returns top-level messages (thread replies excluded), newest first.
func (s *Store) HistoryPage(ctx context.Context, channelID string, hq HistoryQuery) ([]Message, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE channel_id = ? AND thread_ts = ''`
	args := []any{channelID}
	lo, hi := ">", "<"
	if hq.Inclusive {
		lo, hi = ">=", "<="
	}
	if hq.Before != "" {
		q += ` AND ts < ?`
		args = append(args, hq.Before)
	}
	if hq.Latest != "" {
		q += ` AND ts ` + hi + ` ?`
		args = append(args, hq.Latest)
	}
	if hq.Oldest != "" {
		q += ` AND ts ` + lo + ` ?`
		args = append(args, hq.Oldest)
	}
	q, args = withLimit(q+` ORDER BY ts DESC`, args, hq.Limit)
	return query(ctx, s.reader, "history page", q, scanMsg, args...)
}

// MembersOf returns member user ids (sorted) per conversation id.
func (s *Store) MembersOf(ctx context.Context, conversationIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(conversationIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		args = append(args, id)
	}
	type row struct{ conv, user string }
	rows, err := query(ctx, s.reader, "members",
		`SELECT conversation_id, user_id FROM members WHERE conversation_id IN (`+placeholders(len(args))+`) ORDER BY conversation_id, user_id`,
		func(r scanner, v *row) error { return r.Scan(&v.conv, &v.user) }, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.conv] = append(out[r.conv], r.user)
	}
	return out, nil
}

// ThreadPage returns a thread's parent followed by its replies, oldest first.
// afterTS is an exclusive keyset cursor; limit 0 means unlimited.
func (s *Store) ThreadPage(ctx context.Context, channelID, threadTS, afterTS string, limit int) ([]Message, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE channel_id = ? AND (ts = ? OR thread_ts = ?)`
	args := []any{channelID, threadTS, threadTS}
	if afterTS != "" {
		q += ` AND ts > ?`
		args = append(args, afterTS)
	}
	q, args = withLimit(q+` ORDER BY ts`, args, limit)
	return query(ctx, s.reader, "thread page", q, scanMsg, args...)
}

func withLimit(q string, args []any, limit int) (string, []any) {
	if limit > 0 {
		return q + ` LIMIT ?`, append(args, limit)
	}
	return q, args
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
