package web

import (
	"errors"
	"net/http"

	"localslack/internal/service"
)

// statusFor maps a domain error to the HTTP status the UI shows; false for real faults.
func statusFor(err error) (int, string, bool) {
	for _, r := range []struct {
		status int
		errs   []error
	}{
		{http.StatusNotFound, []error{service.ErrChannelNotFound, service.ErrThreadNotFound, service.ErrUserNotFound, service.ErrMessageNotFound}},
		{http.StatusConflict, []error{service.ErrIsArchived, service.ErrAlreadyReacted}},
		{http.StatusForbidden, []error{
			service.ErrNotInChannel, service.ErrUserNotInChannel, service.ErrCantUpdateMessage, service.ErrCantDeleteMessage,
			service.ErrNotAllowedTokenType,
		}},
		{http.StatusBadRequest, []error{
			service.ErrMsgTooLong, service.ErrInvalidBlocks, service.ErrInvalidAttachments, service.ErrInvalidMetadataFormat,
			service.ErrInvalidName, service.ErrInvalidReactionName, service.ErrNoText,
		}},
	} {
		for _, e := range r.errs {
			if errors.Is(err, e) {
				return r.status, http.StatusText(r.status), true
			}
		}
	}
	return 0, "", false
}
