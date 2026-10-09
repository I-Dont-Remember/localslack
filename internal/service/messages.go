package service

import (
	"context"
	"errors"

	"localslack/internal/store"
)

// PostMessage stores a message. A non-empty threadTS replies in that thread;
// replying to a reply attaches to the thread root.
func (s *Service) PostMessage(ctx context.Context, author User, channelID, text, threadTS string) (Message, error) {
	if text == "" {
		return Message{}, ErrNoText
	}
	var msg store.Message
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		conv, err := tx.Conversation(ctx, channelID)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if conv.IsArchived {
			return ErrIsArchived
		}
		root, err := threadRoot(ctx, tx, channelID, threadTS)
		if err != nil {
			return err
		}
		msg = store.Message{ChannelID: channelID, TS: s.ts.next(s.now()), UserID: author.ID, Text: text, ThreadTS: root}
		if err := tx.InsertMessage(ctx, msg); err != nil {
			return err
		}
		if root == "" {
			return nil
		}
		return tx.BumpThread(ctx, channelID, root, msg.TS)
	})
	if err != nil {
		return Message{}, err
	}
	return msgFrom(msg), nil
}

// threadRoot resolves threadTS to the root message's ts ("" for a top-level post).
func threadRoot(ctx context.Context, tx *store.Tx, channelID, threadTS string) (string, error) {
	if threadTS == "" {
		return "", nil
	}
	parent, err := tx.Message(ctx, channelID, threadTS)
	if err != nil {
		return "", notFoundAs(err, ErrThreadNotFound)
	}
	if parent.ThreadTS != "" {
		return parent.ThreadTS, nil
	}
	return parent.TS, nil
}

// History returns a channel's top-level messages, newest first.
func (s *Service) History(ctx context.Context, _ User, channelID string, o PageOpts) (Page[Message], error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
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
	return mapPage(paginate(cursorHistory, items, limit, func(m store.Message) string { return m.TS }), msgFrom), nil
}

// Replies returns a thread (root first, then replies oldest first).
func (s *Service) Replies(ctx context.Context, _ User, channelID, threadTS string, o PageOpts) (Page[Message], error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
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
	return mapPage(page, func(m store.Message) Message {
		out := msgFrom(m)
		if m.ThreadTS != "" {
			out.ParentUserID = root.UserID
		}
		return out
	}), nil
}

func (s *Service) requireChannel(ctx context.Context, channelID string) error {
	if _, err := s.st.Conversation(ctx, channelID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrChannelNotFound
		}
		return err
	}
	return nil
}
