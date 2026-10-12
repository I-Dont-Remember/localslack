package api

import "localslack/internal/service"

// Error codes owned by the message write/react stream.
func messageErrorCodes() map[error]string {
	return map[error]string{
		service.ErrMessageNotFound:     "message_not_found",
		service.ErrCantUpdateMessage:   "cant_update_message",
		service.ErrCantDeleteMessage:   "cant_delete_message",
		service.ErrAlreadyReacted:      "already_reacted",
		service.ErrInvalidReactionName: "invalid_name",
	}
}
