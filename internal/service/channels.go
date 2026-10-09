package service

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"localslack/internal/store"
)

const maxGroupMembers = 8

var channelName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

// CreateChannel makes a public channel with creator as its first member.
func (s *Service) CreateChannel(ctx context.Context, creator User, name string) (Conversation, error) {
	if !channelName.MatchString(name) {
		return Conversation{}, ErrInvalidName
	}
	team, err := s.st.Team(ctx)
	if err != nil {
		return Conversation{}, err
	}
	c := Conversation{
		ID: s.ids.Channel(), TeamID: team.ID, Name: name, Kind: KindChannel,
		Creator: creator.ID, Created: s.now().Unix(),
	}
	err = s.st.Write(ctx, func(tx *store.Tx) error {
		if err := tx.InsertConversation(ctx, c); err != nil {
			return err
		}
		return tx.AddMember(ctx, c.ID, creator.ID)
	})
	if errors.Is(err, store.ErrDuplicate) {
		return Conversation{}, ErrNameTaken
	}
	return c, err
}

// ListOpts filters conversations.list. Types uses Slack names
// (public_channel, private_channel, im, mpim); empty means public_channel.
type ListOpts struct {
	Types           []string
	ExcludeArchived bool
	Cursor          string
	Limit           int
}

// ListConversations lists channels plus the caller's own DMs, paginated by id.
func (s *Service) ListConversations(ctx context.Context, caller User, o ListOpts) (Page[Conversation], error) {
	kinds, err := kindsFor(o.Types)
	if err != nil {
		return Page[Conversation]{}, err
	}
	after, err := decodeCursor(o.Cursor)
	if err != nil {
		return Page[Conversation]{}, err
	}
	if len(kinds) == 0 { // e.g. only private_channel, which LocalSlack does not have
		return Page[Conversation]{}, nil
	}
	limit := clampLimit(o.Limit)
	items, err := s.st.ConversationsPage(ctx, store.ConvQuery{
		Kinds: kinds, ExcludeArchived: o.ExcludeArchived, MemberID: caller.ID, AfterID: after, Limit: limit + 1,
	})
	if err != nil {
		return Page[Conversation]{}, err
	}
	return paginate(items, limit, func(c Conversation) string { return c.ID }), nil
}

func kindsFor(types []string) ([]string, error) {
	if len(types) == 0 {
		types = []string{"public_channel"}
	}
	var kinds []string
	for _, t := range types {
		switch t {
		case "public_channel":
			kinds = append(kinds, KindChannel)
		case "im":
			kinds = append(kinds, KindIM)
		case "mpim":
			kinds = append(kinds, KindMPIM)
		case "private_channel":
		default:
			return nil, ErrInvalidTypes
		}
	}
	return kinds, nil
}

// OpenDM opens (or returns the existing) DM between caller and userIDs:
// an im for one other user (or self), an mpim for 2+ others.
func (s *Service) OpenDM(ctx context.Context, caller User, userIDs []string) (Conversation, error) {
	if len(userIDs) == 0 {
		return Conversation{}, ErrNoUsers
	}
	members := slices.Compact(slices.Sorted(slices.Values(append([]string{caller.ID}, userIDs...))))
	if len(members) > maxGroupMembers {
		return Conversation{}, ErrTooManyUsers
	}
	kind := KindIM
	if len(members) > 2 {
		kind = KindMPIM
	}
	team, err := s.st.Team(ctx)
	if err != nil {
		return Conversation{}, err
	}
	var out Conversation
	err = s.st.Write(ctx, func(tx *store.Tx) error {
		for _, id := range members {
			if _, err := tx.User(ctx, id); err != nil {
				return notFoundAs(err, ErrUserNotFound)
			}
		}
		existing, err := tx.ConversationWithMembers(ctx, kind, members)
		if err != nil {
			return err
		}
		if existing != "" {
			out, err = tx.Conversation(ctx, existing)
			return err
		}
		out = Conversation{
			ID: s.newDMID(kind), TeamID: team.ID, Kind: kind, Creator: caller.ID, Created: s.now().Unix(),
		}
		if err := tx.InsertConversation(ctx, out); err != nil {
			return err
		}
		for _, id := range members {
			if err := tx.AddMember(ctx, out.ID, id); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) newDMID(kind string) string {
	if kind == KindMPIM {
		return s.ids.Group()
	}
	return s.ids.DM()
}

// notFoundAs maps store.ErrNotFound to a domain error, passing other errors through.
func notFoundAs(err, domain error) error {
	if errors.Is(err, store.ErrNotFound) {
		return domain
	}
	return err
}
