package service

import (
	"context"

	"localslack/internal/store"
)

// UpdateOpts describes an edit. Non-empty Content fields replace the stored ones;
// an edit that changes nothing is ErrNoText.
type UpdateOpts struct {
	Channel, TS string
	Content     Content
}

// UpdateMessage edits a message; only its author may. Emits MessageChanged.
func (s *Service) UpdateMessage(ctx context.Context, actor User, o UpdateOpts) (Message, error) {
	c := o.Content
	if c.Text == "" && len(c.Blocks) == 0 && len(c.Attachments) == 0 {
		return Message{}, ErrNoText
	}
	var out Message
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		conv, err := tx.Conversation(ctx, o.Channel)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if err := s.checkWrite(ctx, actor, conv); err != nil {
			return err
		}
		cur, err := tx.Message(ctx, o.Channel, o.TS)
		if err != nil {
			return notFoundAs(err, ErrMessageNotFound)
		}
		if cur.UserID != actor.ID || cur.Subtype == subtypeTombstone {
			return ErrCantUpdateMessage
		}
		next := cur
		if c.Text != "" {
			next.Text = c.Text
		}
		if len(c.Blocks) > 0 {
			next.Blocks = c.Blocks
		}
		if len(c.Attachments) > 0 {
			next.Attachments = c.Attachments
		}
		next.Edited = &store.Edit{User: actor.ID, TS: s.ts.next(s.now())}
		if err := tx.UpdateMessage(ctx, next); err != nil {
			return err
		}
		scope, err := scopeOf(ctx, tx, conv)
		if err != nil {
			return err
		}
		out = msgFrom(next)
		publishAfterCommit(s, tx, MessageChanged{Msg: out, Prev: msgFrom(cur), Scope: scope})
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	return out, nil
}

// subtypeTombstone marks a deleted thread root that still has replies.
const (
	subtypeTombstone = "tombstone"
	tombstoneText    = "This message was deleted."
)

// DeleteMessage deletes a message; only its author may. A reply updates its
// root's counters; a root that still has replies becomes a tombstone (and goes
// once its last reply does). Emits MessageDeleted.
func (s *Service) DeleteMessage(ctx context.Context, actor User, channelID, ts string) error {
	return s.st.Write(ctx, func(tx *store.Tx) error {
		conv, err := tx.Conversation(ctx, channelID)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if err := s.checkAccess(ctx, actor, conv); err != nil {
			return err
		}
		cur, err := tx.Message(ctx, channelID, ts)
		if err != nil {
			return notFoundAs(err, ErrMessageNotFound)
		}
		if cur.Subtype == subtypeTombstone {
			return ErrMessageNotFound
		}
		if cur.UserID != actor.ID {
			return ErrCantDeleteMessage
		}
		if err := removeMessage(ctx, tx, cur); err != nil {
			return err
		}
		if err := tx.DeleteReactions(ctx, channelID, ts); err != nil {
			return err
		}
		scope, err := scopeOf(ctx, tx, conv)
		if err != nil {
			return err
		}
		publishAfterCommit(s, tx, MessageDeleted{Msg: msgFrom(cur), Scope: scope})
		return nil
	})
}

// removeMessage applies the delete rules for cur inside tx.
func removeMessage(ctx context.Context, tx *store.Tx, cur store.Message) error {
	switch {
	case cur.ThreadTS != "":
		return removeReply(ctx, tx, cur)
	case cur.ReplyCount > 0:
		cur.Text, cur.Subtype, cur.Edited = tombstoneText, subtypeTombstone, nil
		cur.Blocks, cur.Attachments, cur.Metadata = nil, nil, nil
		return tx.UpdateMessage(ctx, cur)
	}
	return tx.DeleteMessage(ctx, cur.ChannelID, cur.TS)
}

func removeReply(ctx context.Context, tx *store.Tx, reply store.Message) error {
	if err := tx.DeleteMessage(ctx, reply.ChannelID, reply.TS); err != nil {
		return err
	}
	if err := tx.RecountThread(ctx, reply.ChannelID, reply.ThreadTS); err != nil {
		return err
	}
	root, err := tx.Message(ctx, reply.ChannelID, reply.ThreadTS)
	if err != nil {
		return err
	}
	if root.Subtype == subtypeTombstone && root.ReplyCount == 0 {
		return tx.DeleteMessage(ctx, root.ChannelID, root.TS)
	}
	return nil
}
