package service

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"localslack/internal/store"
)

const maxGroupMembers = 8

const maxChannelNameLen = 80

var (
	channelNameChars = regexp.MustCompile(`^[a-z0-9_-]+$`)
	channelNameStart = regexp.MustCompile(`^[a-z0-9]`)
)

// validateChannelName returns the Slack-specific invalid_name_* error, or nil.
func validateChannelName(name string) error {
	switch {
	case name == "":
		return ErrInvalidNameRequired
	case len(name) > maxChannelNameLen:
		return ErrInvalidNameMaxLength
	case !channelNameChars.MatchString(name):
		return ErrInvalidNameSpecials
	case !channelNameStart.MatchString(name):
		return ErrInvalidNamePunctuation
	}
	return nil
}

// CreateOpt tweaks CreateChannel.
type CreateOpt func(*store.Conversation)

// CreatePrivate makes the channel private: only members see it.
func CreatePrivate() CreateOpt { return func(c *store.Conversation) { c.IsPrivate = true } }

// CreateChannel makes a channel (public unless CreatePrivate) with creator as its first member.
func (s *Service) CreateChannel(ctx context.Context, creator User, name string, opts ...CreateOpt) (Conversation, error) {
	if err := validateChannelName(name); err != nil {
		return Conversation{}, err
	}
	team, err := s.st.Team(ctx)
	if err != nil {
		return Conversation{}, err
	}
	c := store.Conversation{
		ID: s.ids.Channel(), TeamID: team.ID, Name: name, Kind: KindChannel,
		Creator: creator.ID, Created: s.now().Unix(),
	}
	for _, o := range opts {
		o(&c)
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
	if err != nil {
		return Conversation{}, err
	}
	return s.enrichOne(ctx, creator, c)
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
	kinds, privacy, err := kindsFor(o.Types)
	if err != nil {
		return Page[Conversation]{}, err
	}
	after, err := decodeCursor(cursorConversations, o.Cursor)
	if err != nil {
		return Page[Conversation]{}, err
	}
	limit := clampLimit(o.Limit)
	items, err := s.st.ConversationsPage(ctx, store.ConvQuery{
		Kinds: kinds, Privacy: privacy, ExcludeArchived: o.ExcludeArchived, MemberID: caller.ID, AfterID: after, Limit: limit + 1,
	})
	if err != nil {
		return Page[Conversation]{}, err
	}
	page := paginate(cursorConversations, items, limit, func(c store.Conversation) string { return c.ID })
	enriched, err := s.enrich(ctx, caller, page.Items)
	if err != nil {
		return Page[Conversation]{}, err
	}
	return Page[Conversation]{Items: enriched, NextCursor: page.NextCursor}, nil
}

// kindsFor maps Slack conversation types to store kinds plus a channel privacy
// filter ("" when both public and private channels are wanted).
func kindsFor(types []string) (kinds []string, privacy string, err error) {
	if len(types) == 0 {
		types = []string{"public_channel"}
	}
	var pub, priv bool
	for _, t := range types {
		switch t {
		case "public_channel":
			pub = true
		case "private_channel":
			priv = true
		case "im":
			kinds = append(kinds, KindIM)
		case "mpim":
			kinds = append(kinds, KindMPIM)
		default:
			return nil, "", ErrInvalidTypes
		}
	}
	switch {
	case pub && priv:
		kinds = append(kinds, KindChannel)
	case pub:
		kinds, privacy = append(kinds, KindChannel), "public"
	case priv:
		kinds, privacy = append(kinds, KindChannel), "private"
	}
	return kinds, privacy, nil
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
	var out store.Conversation
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
		out = store.Conversation{
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
	if err != nil {
		return Conversation{}, err
	}
	return s.enrichOne(ctx, caller, out)
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

// ArchiveChannel archives a public channel (never #general).
func (s *Service) ArchiveChannel(ctx context.Context, caller User, id string) error {
	return s.st.Write(ctx, func(tx *store.Tx) error {
		c, err := tx.Conversation(ctx, id)
		if err != nil || c.Kind != KindChannel {
			return notFoundOr(err, ErrChannelNotFound)
		}
		if _, err := s.canSee(ctx, caller, c); err != nil {
			return err
		}
		switch {
		case c.Name == "general":
			return ErrCantArchiveGeneral
		case c.IsArchived:
			return ErrAlreadyArchived
		}
		return tx.SetArchived(ctx, id, true)
	})
}

// notFoundOr maps any lookup failure or kind mismatch to domain; real faults pass through.
func notFoundOr(err, domain error) error {
	if err == nil || errors.Is(err, store.ErrNotFound) {
		return domain
	}
	return err
}

// ListAll returns every conversation of the given Slack types visible to caller
// (all pages), ordered by id. For UIs that need the whole list.
func (s *Service) ListAll(ctx context.Context, caller User, types []string, excludeArchived bool) ([]Conversation, error) {
	var all []Conversation
	cursor := ""
	for {
		page, err := s.ListConversations(ctx, caller, ListOpts{
			Types: types, ExcludeArchived: excludeArchived, Cursor: cursor, Limit: maxLimit,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if cursor = page.NextCursor; cursor == "" {
			return all, nil
		}
	}
}
