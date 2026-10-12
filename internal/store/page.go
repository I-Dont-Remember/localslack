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

func withLimit(q string, args []any, limit int) (string, []any) {
	if limit > 0 {
		return q + ` LIMIT ?`, append(args, limit)
	}
	return q, args
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
