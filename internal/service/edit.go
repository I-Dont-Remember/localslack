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
		if err := s.checkAccess(ctx, actor, conv); err != nil {
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
const subtypeTombstone = "tombstone"
