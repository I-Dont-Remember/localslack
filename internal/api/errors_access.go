package api

import "localslack/internal/service"

// Error codes owned by the access (membership, token type) stream.
func accessErrorCodes() map[error]string {
	return map[error]string{
		service.ErrNotInChannel:        "not_in_channel",
		service.ErrUserNotInChannel:    "user_not_in_channel",
		service.ErrNotAllowedTokenType: "not_allowed_token_type",
	}
}
