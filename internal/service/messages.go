package service

import (
	"context"

	"localslack/internal/store"
)

// History returns a channel's top-level messages, newest first.
func (s *Service) History(ctx context.Context, u User, channelID string, o PageOpts) (Page[Message], error) {
	if err := s.requireReadable(ctx, u, channelID); err != nil {
		return Page[Message]{}, err
	}
	before, err := decodeCursor(cursorHistory, o.Cursor)
	if err != nil {
		return Page[Message]{}, err
	}
	limit := clampLimit(o.Limit)
	items, err := s.st.HistoryPage(ctx, channelID, store.HistoryQuery{
		Before: before, Oldest: o.Oldest, Latest: o.Latest, Inclusive: o.Inclusive, Limit: limit + 1,
	})
	if err != nil {
		return Page[Message]{}, err
	}
	out := mapPage(paginate(cursorHistory, items, limit, func(m store.Message) string { return m.TS }), msgFrom)
	if err := s.withReactions(ctx, channelID, out.Items); err != nil {
		return Page[Message]{}, err
	}
	return out, nil
}

// Replies returns a thread (root first, then replies oldest first).
func (s *Service) Replies(ctx context.Context, u User, channelID, threadTS string, o PageOpts) (Page[Message], error) {
	if err := s.requireReadable(ctx, u, channelID); err != nil {
		return Page[Message]{}, err
	}
	after, err := decodeCursor(cursorReplies, o.Cursor)
	if err != nil {
		return Page[Message]{}, err
	}
	m, err := s.st.Message(ctx, channelID, threadTS)
	if err != nil {
		return Page[Message]{}, notFoundAs(err, ErrThreadNotFound)
	}
	rootTS := threadTS
	if m.ThreadTS != "" {
		rootTS = m.ThreadTS
	}
	root, err := s.st.Message(ctx, channelID, rootTS)
	if err != nil {
		return Page[Message]{}, notFoundAs(err, ErrThreadNotFound)
	}
	limit := clampLimit(o.Limit)
	items, err := s.st.ThreadPage(ctx, channelID, rootTS, after, limit+1)
	if err != nil {
		return Page[Message]{}, err
	}
	page := paginate(cursorReplies, items, limit, func(m store.Message) string { return m.TS })
	out := mapPage(page, func(m store.Message) Message {
		msg := msgFrom(m)
		if m.ThreadTS != "" {
			msg.ParentUserID = root.UserID
		}
		return msg
	})
	if err := s.withReactions(ctx, channelID, out.Items); err != nil {
		return Page[Message]{}, err
	}
	return out, nil
}

func (s *Service) requireReadable(ctx context.Context, u User, channelID string) error {
	c, err := s.st.Conversation(ctx, channelID)
	if err != nil {
		return notFoundAs(err, ErrChannelNotFound)
	}
	return s.canRead(ctx, u, c)
}

func (s *Service) requireVisible(ctx context.Context, u User, channelID string) error {
	c, err := s.st.Conversation(ctx, channelID)
	if err != nil {
		return notFoundAs(err, ErrChannelNotFound)
	}
	_, err = s.canSee(ctx, u, c)
	return err
}
