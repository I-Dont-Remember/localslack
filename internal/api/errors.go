package api

import (
	"errors"

	"localslack/internal/service"
)

// ErrorCode maps a domain error to its Slack error code.
func ErrorCode(err error) (string, bool) {
	table := []struct {
		err  error
		code string
	}{
		{service.ErrNotAuthed, "not_authed"},
		{service.ErrInvalidAuth, "invalid_auth"},
		{service.ErrNameTaken, "name_taken"},
		{service.ErrInvalidName, "invalid_name"},
		{service.ErrChannelNotFound, "channel_not_found"},
		{service.ErrUserNotFound, "user_not_found"},
		{service.ErrThreadNotFound, "thread_not_found"},
		{service.ErrNoText, "no_text"},
		{service.ErrNoUsers, "users_list_not_supplied"},
		{service.ErrTooManyUsers, "too_many_users"},
		{service.ErrInvalidCursor, "invalid_cursor"},
		{service.ErrInvalidTypes, "invalid_types"},
		{service.ErrIsArchived, "is_archived"},
	}
	for _, e := range table {
		if errors.Is(err, e.err) {
			return e.code, true
		}
	}
	return "", false
}
