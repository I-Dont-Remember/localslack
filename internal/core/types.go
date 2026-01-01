package core

type WorkspaceID string
type TeamID string
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

type SlackUser struct {
	ID        UserID           `json:"id"`
	TeamID    TeamID           `json:"team_id"`
	Name      string           `json:"name"`
	Deleted   bool             `json:"deleted"`
	Profile   SlackUserProfile `json:"profile"`
	IsAdmin   bool             `json:"is_admin"`
	IsOwner   bool             `json:"is_owner"`
	IsBot     bool             `json:"is_bot"`
	IsAppUser bool             `json:"is_app_user"`
	Tz        string           `json:"tz"`
	Updated   int              `json:"updated"`
}

type SlackUserProfile struct {
	Team                  TeamID `json:"team"`
	RealName              string `json:"real_name"`
	RealNameNormalized    string `json:"real_name_normalized"`
	DisplayName           string `json:"display_name"`
	DisplayNameNormalized string `json:"display_name_normalized"`
	StatusText            string `json:"status_text"`
	StatusEmoji           string `json:"status_emoji"`
	Email                 string `json:"email"`
	Image24               string `json:"image_24"`
	Image32               string `json:"image_32"`
	Image512              string `json:"image_512"`
}
