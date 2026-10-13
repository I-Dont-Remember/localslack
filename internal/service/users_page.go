package service

import (
	"context"
	"slices"
)

const cursorUsers = "u"

// UsersPage lists users by id. Limit <= 0 returns everyone (Slack's users.list default),
// larger limits are capped at maxLimit.
func (s *Service) UsersPage(ctx context.Context, o PageOpts) (Page[User], error) {
	after, err := decodeCursor(cursorUsers, o.Cursor)
	if err != nil {
		return Page[User]{}, err
	}
	users, err := s.st.Users(ctx) // ordered by id
	if err != nil {
		return Page[User]{}, err
	}
	if after != "" {
		i, _ := slices.BinarySearchFunc(users, after, func(u User, id string) int {
			if u.ID <= id {
				return -1
			}
			return 1
		})
		users = users[i:]
	}
	if o.Limit <= 0 {
		return Page[User]{Items: users}, nil
	}
	return paginate(cursorUsers, users, min(o.Limit, maxLimit), func(u User) string { return u.ID }), nil
}
