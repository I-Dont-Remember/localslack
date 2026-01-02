package core

type WorkspaceID string
type TeamID string
type UserID string
type ChannelID string
type MessageID string

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

type SlackConversation struct {
	ID                 ChannelID     `json:"id"`
	Name               string        `json:"name"`
	IsChannel          bool          `json:"is_channel"`
	IsGroup            bool          `json:"is_group"`
	IsIM               bool          `json:"is_im"`
	Created            int           `json:"created"`
	Creator            UserID        `json:"creator"`
	IsArchived         bool          `json:"is_archived"`
	IsGeneral          bool          `json:"is_general"`
	Unlinked           int           `json:"unlinked"`
	NameNormalized     string        `json:"name_normalized"`
	IsShared           bool          `json:"is_shared"`
	IsExtShared        bool          `json:"is_ext_shared"`
	IsOrgShared        bool          `json:"is_org_shared"`
	PendingShared      []UserID      `json:"pending_shared"`
	IsPendingExtShared bool          `json:"is_pending_ext_shared"`
	IsMember           bool          `json:"is_member"`
	IsPrivate          bool          `json:"is_private"`
	IsMpim             bool          `json:"is_mpim"`
	IsOpen             bool          `json:"is_open"`
	Updated            int           `json:"updated"`
	Topic              *SlackTopic   `json:"topic"`
	Purpose            *SlackPurpose `json:"purpose"`
	Priority           int           `json:"priority"`
}

type SlackTopic struct {
	Value   string `json:"value"`
	Creator UserID `json:"creator"`
	LastSet int    `json:"last_set"`
}

type SlackPurpose struct {
	Value   string `json:"value"`
	Creator UserID `json:"creator"`
	LastSet int    `json:"last_set"`
}

type SlackMessage struct {
	// TODO: handle attachments, rich text, etc other crap
	Text     string `json:"text"`
	Username string `json:"username"`
	User     UserID `json:"user",omitempty`
	Type     string `json:"type"`
	SubType  string `json:"subtype"`
	Ts       string `json:"ts"`
}
