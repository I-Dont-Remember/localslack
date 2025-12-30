package constants

const (
	CONTENT_TYPE_JSON = "application/json"
	CONTENT_TYPE_FORM = "application/x-www-form-urlencoded"
	CONTENT_TYPE_TEXT = "text/plain"
	CONTENT_TYPE_HTML = "text/html"
)

var ALLOWED_CONTENT_TYPES = []string{
	CONTENT_TYPE_JSON,
	CONTENT_TYPE_FORM,
}
