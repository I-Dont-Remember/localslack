// Access rules (membership, private channels, scope gating) live here as one
// rule set; every service method asks it before touching a conversation.
//
//	canSee    visibility: private channels and DMs hide from non-members (channel_not_found)
//	canRead   see + a bot must be a member of a public channel (not_in_channel)
//	canPost   see + a non-member needs chat:write.public (bots only); humans must be members
//	canReact  see + membership for everyone
//
// Callers are Users, not Principals: a bot user's scopes come from its app
// (or the default bot scopes), a human is a user token or the web UI. The UI
// joins the actor to a public channel before posting (EnsureMember).
package service

import (
	"context"
	"slices"

	"localslack/internal/store"
)

const scopeWritePublic = "chat:write.public"

// restricted reports whether only members may see the conversation.
func restricted(c store.Conversation) bool { return c.IsPrivate || c.Kind != KindChannel }

func (s *Service) isMember(ctx context.Context, convID, userID string) (bool, error) {
	byConv, err := s.st.MembersOf(ctx, []string{convID})
	if err != nil {
		return false, err
	}
	return slices.Contains(byConv[convID], userID), nil
}

// botScopes are the scopes of the app that owns bot user u; the default bot has the default scopes.
func (s *Service) botScopes(u User) []string {
	for _, a := range s.apps.all() {
		if a.BotUserID == u.ID {
			return a.Scopes
		}
	}
	return DefaultBotScopes()
}

func (s *Service) canSee(ctx context.Context, u User, c store.Conversation) (member bool, err error) {
	member, err = s.isMember(ctx, c.ID, u.ID)
	if err != nil {
		return false, err
	}
	if restricted(c) && !member {
		return false, ErrChannelNotFound
	}
	return member, nil
}

func (s *Service) canRead(ctx context.Context, u User, c store.Conversation) error {
	member, err := s.canSee(ctx, u, c)
	if err != nil {
		return err
	}
	if u.IsBot && !member {
		return ErrNotInChannel
	}
	return nil
}

// checkAccess is the post/update/delete/ephemeral rule (canPost).
func (s *Service) checkAccess(ctx context.Context, u User, c store.Conversation) error {
	member, err := s.canSee(ctx, u, c)
	if err != nil {
		return err
	}
	if member || (u.IsBot && slices.Contains(s.botScopes(u), scopeWritePublic)) {
		return nil
	}
	return ErrNotInChannel
}

func (s *Service) canReact(ctx context.Context, u User, c store.Conversation) error {
	member, err := s.canSee(ctx, u, c)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotInChannel
	}
	return nil
}

// checkWrite is canPost plus the archived rule, for methods that add or change content.
func (s *Service) checkWrite(ctx context.Context, u User, c store.Conversation) error {
	if err := s.checkAccess(ctx, u, c); err != nil {
		return err
	}
	if c.IsArchived {
		return ErrIsArchived
	}
	return nil
}

// EnsureMember joins u to a public, unarchived channel (a no-op elsewhere).
// For the web UI, which acts as a human who may post anywhere public; the API
// has no join primitive in this tier.
func (s *Service) EnsureMember(ctx context.Context, u User, channelID string) error {
	return s.st.Write(ctx, func(tx *store.Tx) error {
		c, err := tx.Conversation(ctx, channelID)
		if err != nil {
			return notFoundAs(err, ErrChannelNotFound)
		}
		if restricted(c) || c.IsArchived {
			return nil
		}
		if member, err := s.isMember(ctx, c.ID, u.ID); err != nil || member {
			return err
		}
		return tx.AddMember(ctx, c.ID, u.ID)
	})
}
