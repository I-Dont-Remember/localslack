package service

import (
	"context"
	"errors"
	"regexp"

	"localslack/internal/store"
)

// reactionName is an emoji short name without colons, optionally with a skin tone.
var reactionName = regexp.MustCompile(`^[a-z0-9_+\-]{1,100}(::skin-tone-[2-6])?$`)

// AddReaction adds actor's emoji reaction to a message. Emits ReactionAdded.
func (s *Service) AddReaction(ctx context.Context, actor User, channelID, ts, name string) error {
	if !reactionName.MatchString(name) {
		return ErrInvalidReactionName
	}
	return s.st.Write(ctx, func(tx *store.Tx) error {
		conv, err := tx.Conversation(ctx, channelID)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if err := s.checkAccess(ctx, actor, conv); err != nil {
			return err
		}
		m, err := tx.Message(ctx, channelID, ts)
		if err != nil {
			return notFoundAs(err, ErrMessageNotFound)
		}
		if m.Subtype == subtypeTombstone {
			return ErrMessageNotFound
		}
		err = tx.InsertReaction(ctx, store.Reaction{
			ChannelID: channelID, TS: ts, Name: name, UserID: actor.ID, Created: s.now().Unix(),
		})
		if errors.Is(err, store.ErrDuplicate) {
			return ErrAlreadyReacted
		}
		if err != nil {
			return err
		}
		rs, err := tx.ReactionsOf(ctx, channelID, ts)
		if err != nil {
			return err
		}
		scope, err := scopeOf(ctx, tx, conv)
		if err != nil {
			return err
		}
		msg := msgFrom(m)
		msg.Reactions = groupReactions(rs)
		publishAfterCommit(s, tx, ReactionAdded{User: actor.ID, Reaction: name, Msg: msg, Scope: scope})
		return nil
	})
}

// groupReactions folds per-user rows into per-emoji entries, in first-added order.
func groupReactions(rs []store.Reaction) []Reaction {
	var out []Reaction
	idx := map[string]int{}
	for _, r := range rs {
		i, ok := idx[r.Name]
		if !ok {
			i = len(out)
			idx[r.Name] = i
			out = append(out, Reaction{Name: r.Name})
		}
		out[i].Users = append(out[i].Users, r.UserID)
		out[i].Count++
	}
	return out
}

// withReactions fills Reactions on msgs (all in channelID) in place.
func (s *Service) withReactions(ctx context.Context, channelID string, msgs []Message) error {
	tss := make([]string, 0, len(msgs))
	for _, m := range msgs {
		tss = append(tss, m.TS)
	}
	byTS, err := s.st.ReactionsFor(ctx, channelID, tss)
	if err != nil {
		return err
	}
	for i := range msgs {
		msgs[i].Reactions = groupReactions(byTS[msgs[i].TS])
	}
	return nil
}
