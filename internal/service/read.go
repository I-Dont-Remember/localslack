package service

import (
	"context"
	"slices"
)

const cursorMembers = "m"

// User returns one user by id.
func (s *Service) User(ctx context.Context, id string) (User, error) {
	u, err := s.st.User(ctx, id)
	if err != nil {
		return User{}, notFoundAs(err, ErrUserNotFound)
	}
	return u, nil
}

// Members pages a conversation's member ids, ordered by id.
func (s *Service) Members(ctx context.Context, u User, channelID string, o PageOpts) (Page[string], error) {
	if err := s.requireVisible(ctx, u, channelID); err != nil {
		return Page[string]{}, err
	}
	after, err := decodeCursor(cursorMembers, o.Cursor)
	if err != nil {
		return Page[string]{}, err
	}
	byConv, err := s.st.MembersOf(ctx, []string{channelID})
	if err != nil {
		return Page[string]{}, err
	}
	ids := byConv[channelID] // sorted by the store
	if after != "" {
		i, found := slices.BinarySearch(ids, after)
		if found {
			i++
		}
		ids = ids[i:]
	}
	return paginate(cursorMembers, ids, clampLimit(o.Limit), func(id string) string { return id }), nil
}
