package api

import (
	"errors"
	"maps"
	"reflect"

	"localslack/internal/service"
)

// errInvalidArguments marks unparseable request params/bodies (API-level, not a domain error).
var errInvalidArguments = errors.New("invalid arguments")

// errorCodes merges every stream's error->code table. A stream adds its codes in
// its own errors_<stream>.go and one line here; sentinels must be comparable.
func errorCodes() map[error]string {
	out := map[error]string{}
	for _, part := range []map[error]string{
		coreErrorCodes(), messageErrorCodes(), contentErrorCodes(), accessErrorCodes(), nameErrorCodes(),
	} {
		maps.Copy(out, part)
	}
	return out
}

func coreErrorCodes() map[error]string {
	return map[error]string{
		errInvalidArguments:           "invalid_arguments",
		service.ErrNotAuthed:          "not_authed",
		service.ErrInvalidAuth:        "invalid_auth",
		service.ErrNameTaken:          "name_taken",
		service.ErrInvalidName:        "invalid_name",
		service.ErrChannelNotFound:    "channel_not_found",
		service.ErrUserNotFound:       "user_not_found",
		service.ErrThreadNotFound:     "thread_not_found",
		service.ErrNoText:             "no_text",
		service.ErrNoUsers:            "users_list_not_supplied",
		service.ErrTooManyUsers:       "too_many_users",
		service.ErrInvalidCursor:      "invalid_cursor",
		service.ErrInvalidTypes:       "invalid_types",
		service.ErrIsArchived:         "is_archived",
		service.ErrAlreadyArchived:    "already_archived",
		service.ErrCantArchiveGeneral: "cant_archive_general",
	}
}

// ErrorCode maps a domain error to its Slack error code. The most specific
// error in err's chain wins, so a child of ErrInvalidName keeps its own code.
func ErrorCode(err error) (string, bool) {
	return lookupCode(errorCodes(), err)
}

func lookupCode(codes map[error]string, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if reflect.TypeOf(err).Comparable() {
		if code, ok := codes[err]; ok {
			return code, true
		}
	}
	switch e := err.(type) { //nolint:errorlint // walking the chain by hand to prefer the outermost match
	case interface{ Unwrap() error }:
		return lookupCode(codes, e.Unwrap())
	case interface{ Unwrap() []error }:
		for _, c := range e.Unwrap() {
			if code, ok := lookupCode(codes, c); ok {
				return code, true
			}
		}
	}
	return "", false
}
