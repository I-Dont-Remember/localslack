package service

import "errors"

// Domain errors. api/web translate these into Slack error codes / HTML.
var (
	ErrNotAuthed       = errors.New("not authed")
	ErrInvalidAuth     = errors.New("invalid auth")
	ErrNameTaken       = errors.New("name taken")
	ErrInvalidName     = errors.New("invalid name")
	ErrChannelNotFound = errors.New("channel not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrThreadNotFound  = errors.New("thread not found")
	ErrNoText          = errors.New("no text")
	ErrNoUsers         = errors.New("no users supplied")
	ErrTooManyUsers    = errors.New("too many users")
	ErrInvalidCursor   = errors.New("invalid cursor")
	ErrInvalidTypes    = errors.New("invalid types")
	ErrIsArchived      = errors.New("channel is archived")
)
