package service

import (
	"context"
	"encoding/json"

	"localslack/internal/store"
)

// Content is a message body. Text is required today; blocks-only posts arrive with
// the content stream.
type Content struct {
	Text                          string
	Blocks, Attachments, Metadata json.RawMessage
}

// PostOpts describes one message to post. A non-empty ThreadTS replies in that
// thread; replying to a reply attaches to the thread root.
type PostOpts struct {
	Channel, ThreadTS string
	Content           Content
}

// PostMessage stores a message authored by author.
func (s *Service) PostMessage(ctx context.Context, author User, o PostOpts) (Message, error) {
	if o.Content.Text == "" {
		return Message{}, ErrNoText
	}
	var out Message
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		conv, err := tx.Conversation(ctx, o.Channel)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if err := s.checkAccess(ctx, author, conv); err != nil {
			return err
		}
		if conv.IsArchived {
			return ErrIsArchived
		}
		root, err := threadRoot(ctx, tx, o.Channel, o.ThreadTS)
		if err != nil {
			return err
		}
		msg := store.Message{
			ChannelID: o.Channel, TS: s.ts.next(s.now()), UserID: author.ID, Text: o.Content.Text, ThreadTS: root,
			BotID: author.BotID, Blocks: o.Content.Blocks, Attachments: o.Content.Attachments, Metadata: o.Content.Metadata,
		}
		if err := tx.InsertMessage(ctx, msg); err != nil {
			return err
		}
		if root != "" {
			if err := tx.BumpThread(ctx, o.Channel, root, msg.TS); err != nil {
				return err
			}
		}
		scope, err := scopeOf(ctx, tx, conv)
		if err != nil {
			return err
		}
		out = msgFrom(msg)
		publishAfterCommit(s, tx, MessagePosted{Msg: out, Scope: scope})
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	return out, nil
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
