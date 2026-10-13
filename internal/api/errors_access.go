package api

import "localslack/internal/service"

// Error codes owned by the access (membership, token type) stream.
func accessErrorCodes() map[error]string {
	return map[error]string{
		service.ErrNotInChannel:        "not_in_channel",
		service.ErrUserNotInChannel:    "user_not_in_channel",
		service.ErrNotAllowedTokenType: "not_allowed_token_type",

		service.ErrAlreadyInChannel:       "already_in_channel",
		service.ErrCantInviteSelf:         "cant_invite_self",
		service.ErrCantLeaveGeneral:       "cant_leave_general",
		service.ErrCantKickSelf:           "cant_kick_self",
		service.ErrCantKickFromGeneral:    "cant_kick_from_general",
		service.ErrUnsupportedChannelType: "method_not_supported_for_channel_type",
	}
}
