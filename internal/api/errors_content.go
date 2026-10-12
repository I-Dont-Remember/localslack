package api

import "localslack/internal/service"

// Error codes owned by the blocks/attachments/metadata stream.
func contentErrorCodes() map[error]string {
	return map[error]string{
		service.ErrInvalidBlocks:         "invalid_blocks",
		service.ErrInvalidAttachments:    "invalid_attachments",
		service.ErrInvalidMetadataFormat: "invalid_metadata_format",
		service.ErrMsgTooLong:            "msg_too_long",
	}
}
