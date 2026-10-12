package api_test

import (
	"fmt"
	"testing"

	"localslack/internal/api"
	"localslack/internal/service"
)

func TestErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want string
	}{
		{service.ErrChannelNotFound, "channel_not_found"},
		{service.ErrInvalidName, "invalid_name"},
		{service.ErrInvalidNameRequired, "invalid_name_required"},
		{service.ErrInvalidNameSpecials, "invalid_name_specials"},
		{service.ErrInvalidNameMaxLength, "invalid_name_maxlength"},
		{service.ErrInvalidNamePunctuation, "invalid_name_punctuation"},
		{fmt.Errorf("create: %w", service.ErrInvalidNameSpecials), "invalid_name_specials"},
		{service.ErrInvalidReactionName, "invalid_name"},
		{service.ErrMessageNotFound, "message_not_found"},
		{service.ErrCantUpdateMessage, "cant_update_message"},
		{service.ErrCantDeleteMessage, "cant_delete_message"},
		{service.ErrAlreadyReacted, "already_reacted"},
		{service.ErrNotInChannel, "not_in_channel"},
		{service.ErrUserNotInChannel, "user_not_in_channel"},
		{service.ErrInvalidBlocks, "invalid_blocks"},
		{service.ErrInvalidAttachments, "invalid_attachments"},
		{service.ErrInvalidMetadataFormat, "invalid_metadata_format"},
		{service.ErrMsgTooLong, "msg_too_long"},
		{service.ErrNotAllowedTokenType, "not_allowed_token_type"},
	}
	for _, tt := range tests {
		t.Run(tt.want+"/"+tt.err.Error(), func(t *testing.T) {
			t.Parallel()
			got, ok := api.ErrorCode(tt.err)
			if !ok || got != tt.want {
				t.Errorf("ErrorCode(%v) = %q, %v; want %q", tt.err, got, ok, tt.want)
			}
		})
	}
	if code, ok := api.ErrorCode(fmt.Errorf("other")); ok {
		t.Errorf("unmapped error mapped to %q", code)
	}
}
