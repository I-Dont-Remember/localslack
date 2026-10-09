// Package constants defines shared application constants.
package constants

// Content type constants for HTTP headers.
const (
	ContentTypeJSON = "application/json"
	ContentTypeForm = "application/x-www-form-urlencoded"
	ContentTypeText = "text/plain"
	ContentTypeHTML = "text/html"
)

// AllowedContentTypes returns the list of accepted request content types.
func AllowedContentTypes() []string {
	return []string{
		ContentTypeJSON,
		ContentTypeForm,
	}
}
