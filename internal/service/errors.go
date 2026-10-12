package service

import (
	"errors"
	"fmt"
)

// Domain errors. api/web translate these into Slack error codes / HTML.
var (
	ErrNotAuthed          = errors.New("not authed")
	ErrInvalidAuth        = errors.New("invalid auth")
	ErrNameTaken          = errors.New("name taken")
	ErrInvalidName        = errors.New("invalid name")
	ErrChannelNotFound    = errors.New("channel not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrThreadNotFound     = errors.New("thread not found")
	ErrNoText             = errors.New("no text")
	ErrNoUsers            = errors.New("no users supplied")
	ErrTooManyUsers       = errors.New("too many users")
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrInvalidTypes       = errors.New("invalid types")
	ErrIsArchived         = errors.New("channel is archived")
	ErrAlreadyArchived    = errors.New("already archived")
	ErrCantArchiveGeneral = errors.New("cannot archive general")
)

// Errors reserved for the test-loop streams; mapped in api/errors_<stream>.go and web/errors.go.
var (
	ErrMessageNotFound       = errors.New("message not found")
	ErrCantUpdateMessage     = errors.New("cannot update message")
	ErrCantDeleteMessage     = errors.New("cannot delete message")
	ErrAlreadyReacted        = errors.New("already reacted")
	ErrInvalidReactionName   = errors.New("invalid reaction name")
	ErrNotInChannel          = errors.New("not in channel")
	ErrUserNotInChannel      = errors.New("user not in channel")
	ErrInvalidBlocks         = errors.New("invalid blocks")
	ErrInvalidAttachments    = errors.New("invalid attachments")
	ErrInvalidMetadataFormat = errors.New("invalid metadata format")
	ErrMsgTooLong            = errors.New("message too long")
	ErrNotAllowedTokenType   = errors.New("token type not allowed")
)

// Channel-name failures; each satisfies errors.Is(err, ErrInvalidName).
var (
	ErrInvalidNameRequired    = fmt.Errorf("%w: required", ErrInvalidName)
	ErrInvalidNameSpecials    = fmt.Errorf("%w: specials", ErrInvalidName)
	ErrInvalidNameMaxLength   = fmt.Errorf("%w: max length", ErrInvalidName)
	ErrInvalidNamePunctuation = fmt.Errorf("%w: punctuation", ErrInvalidName)
)

// AllErrors lists every domain error so adapters can verify they map each one.
func AllErrors() []error {
	return []error{
		ErrNotAuthed, ErrInvalidAuth, ErrNameTaken, ErrInvalidName, ErrChannelNotFound, ErrUserNotFound,
		ErrThreadNotFound, ErrNoText, ErrNoUsers, ErrTooManyUsers, ErrInvalidCursor, ErrInvalidTypes, ErrIsArchived,
		ErrAlreadyArchived, ErrCantArchiveGeneral,
		ErrMessageNotFound, ErrCantUpdateMessage, ErrCantDeleteMessage, ErrAlreadyReacted, ErrInvalidReactionName,
		ErrNotInChannel, ErrUserNotInChannel, ErrInvalidBlocks, ErrInvalidAttachments, ErrInvalidMetadataFormat,
		ErrMsgTooLong, ErrNotAllowedTokenType,
		ErrInvalidNameRequired, ErrInvalidNameSpecials, ErrInvalidNameMaxLength, ErrInvalidNamePunctuation,
	}
}
