package service_test

import (
	"errors"
	"testing"

	"localslack/internal/service"
)

func TestAllErrorsDistinctAndInvalidNameFamily(t *testing.T) {
	t.Parallel()

	seen := map[error]bool{}
	for _, err := range service.AllErrors() {
		if seen[err] {
			t.Errorf("AllErrors lists %q twice", err)
		}
		seen[err] = true
	}
	for _, err := range []error{
		service.ErrMessageNotFound, service.ErrCantUpdateMessage, service.ErrCantDeleteMessage, service.ErrAlreadyReacted,
		service.ErrInvalidReactionName, service.ErrNotInChannel, service.ErrUserNotInChannel, service.ErrInvalidBlocks,
		service.ErrInvalidAttachments, service.ErrInvalidMetadataFormat, service.ErrMsgTooLong, service.ErrNotAllowedTokenType,
		service.ErrInvalidNameRequired, service.ErrInvalidNameSpecials, service.ErrInvalidNameMaxLength, service.ErrInvalidNamePunctuation,
	} {
		if !seen[err] {
			t.Errorf("AllErrors is missing %q", err)
		}
	}
	for _, child := range []error{
		service.ErrInvalidNameRequired, service.ErrInvalidNameSpecials, service.ErrInvalidNameMaxLength, service.ErrInvalidNamePunctuation,
	} {
		if !errors.Is(child, service.ErrInvalidName) {
			t.Errorf("%q should satisfy errors.Is(ErrInvalidName)", child)
		}
	}
}
