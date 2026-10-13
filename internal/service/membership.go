package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"localslack/internal/store"
)

// InviteFailure is why one user in a conversations.invite call was refused.
type InviteFailure struct {
	User string
	Err  error
}

// InviteError is returned when an invite fails for some or all users.
// errors.Is matches the first failure's error (what Slack reports as `error`).
type InviteError struct{ Failures []InviteFailure }

func (e *InviteError) Error() string {
	parts := make([]string, len(e.Failures))
	for i, f := range e.Failures {
		parts[i] = fmt.Sprintf("%s: %v", f.User, f.Err)
	}
	return "invite failed: " + strings.Join(parts, "; ")
}

func (e *InviteError) Unwrap() error { return e.Failures[0].Err }

// memberChannel loads a channel for a membership change by caller: private
// channels and DMs hide from non-members, public ones answer not_in_channel,
// and only channels (not DMs) can change membership.
func memberChannel(ctx context.Context, tx *store.Tx, caller User, id string) (store.Conversation, error) {
	c, err := tx.Conversation(ctx, id)
	if err != nil {
		return c, notFoundAs(err, ErrChannelNotFound)
	}
	member, err := tx.IsMember(ctx, c.ID, caller.ID)
	switch {
	case err != nil:
		return c, err
	case !member && restricted(c):
		return c, ErrChannelNotFound
	case !member:
		return c, ErrNotInChannel
	case c.Kind != KindChannel:
		return c, ErrUnsupportedChannelType
	case c.IsArchived:
		return c, ErrIsArchived
	}
	return c, nil
}

// JoinChannel adds u to a public channel. already reports that u was a member
// (Slack answers ok with an already_in_channel warning). Private channels and
// DMs are hidden from non-members.
func (s *Service) JoinChannel(ctx context.Context, u User, id string) (conv Conversation, already bool, err error) {
	var c store.Conversation
	err = s.st.Write(ctx, func(tx *store.Tx) error {
		var err error
		if c, err = tx.Conversation(ctx, id); err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if already, err = tx.IsMember(ctx, c.ID, u.ID); err != nil {
			return err
		}
		switch {
		case restricted(c) && !already:
			return ErrChannelNotFound
		case c.IsArchived:
			return ErrIsArchived
		case already:
			return nil
		}
		return tx.AddMember(ctx, c.ID, u.ID)
	})
	if err != nil {
		return Conversation{}, false, err
	}
	conv, err = s.enrichOne(ctx, u, c)
	return conv, already, err
}

// LeaveChannel removes u from a channel (never #general).
func (s *Service) LeaveChannel(ctx context.Context, u User, id string) error {
	return s.st.Write(ctx, func(tx *store.Tx) error {
		c, err := memberChannel(ctx, tx, u, id)
		if err != nil {
			return err
		}
		if c.Name == "general" {
			return ErrCantLeaveGeneral
		}
		return tx.RemoveMember(ctx, c.ID, u.ID)
	})
}

// InviteToChannel adds users to a channel the caller is in. Any refused user
// fails the whole call with *InviteError and changes nothing, unless force is
// set and at least one user is valid (then the valid ones are added).
func (s *Service) InviteToChannel(ctx context.Context, caller User, id string, userIDs []string, force bool) (Conversation, error) {
	var c store.Conversation
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		var err error
		if c, err = memberChannel(ctx, tx, caller, id); err != nil {
			return err
		}
		if len(userIDs) == 0 {
			return ErrNoUsers
		}
		var valid []string
		var failures []InviteFailure
		for _, uid := range slices.Compact(slices.Clone(userIDs)) {
			if err := inviteCheck(ctx, tx, caller, c.ID, uid); err != nil {
				failures = append(failures, InviteFailure{User: uid, Err: err})
				continue
			}
			valid = append(valid, uid)
		}
		if len(failures) > 0 && (!force || len(valid) == 0) {
			return &InviteError{Failures: failures}
		}
		for _, uid := range valid {
			if err := tx.AddMember(ctx, c.ID, uid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Conversation{}, err
	}
	return s.enrichOne(ctx, caller, c)
}

// inviteCheck is the per-user rule of conversations.invite; real faults come back as non-domain errors.
func inviteCheck(ctx context.Context, tx *store.Tx, caller User, convID, userID string) error {
	if userID == caller.ID {
		return ErrCantInviteSelf
	}
	if _, err := tx.User(ctx, userID); err != nil {
		return notFoundAs(err, ErrUserNotFound)
	}
	member, err := tx.IsMember(ctx, convID, userID)
	if err != nil {
		return err
	}
	if member {
		return ErrAlreadyInChannel
	}
	return nil
}

// KickFromChannel removes another user from a channel the caller is in (never #general).
func (s *Service) KickFromChannel(ctx context.Context, caller User, id, userID string) error {
	return s.st.Write(ctx, func(tx *store.Tx) error {
		c, err := memberChannel(ctx, tx, caller, id)
		if err != nil {
			return err
		}
		if userID == caller.ID {
			return ErrCantKickSelf
		}
		if _, err := tx.User(ctx, userID); err != nil {
			return notFoundAs(err, ErrUserNotFound)
		}
		if c.Name == "general" {
			return ErrCantKickFromGeneral
		}
		if err := tx.RemoveMember(ctx, c.ID, userID); err != nil {
			return notFoundAs(err, ErrNotInChannel)
		}
		return nil
	})
}
