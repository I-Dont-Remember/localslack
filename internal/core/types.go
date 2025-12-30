package core

type WorkspaceID string
type UserID string
type ChannelID string
type MessageID string

type SlackError struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func NewSlackError(code string) *SlackError {
	return &SlackError{OK: false, Error: code}
}
