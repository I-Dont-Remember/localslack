package api

import "localslack/internal/service"

// Error codes owned by the error-fidelity stream (split invalid_name).
func nameErrorCodes() map[error]string {
	return map[error]string{
		service.ErrInvalidNameRequired:    "invalid_name_required",
		service.ErrInvalidNameSpecials:    "invalid_name_specials",
		service.ErrInvalidNameMaxLength:   "invalid_name_maxlength",
		service.ErrInvalidNamePunctuation: "invalid_name_punctuation",
	}
}
