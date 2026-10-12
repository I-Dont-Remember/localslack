// Package appmanifest holds the Slack app manifest types LocalSlack honors.
// Pure types: parsing, validation and registration live with the apps stream.
package appmanifest

// Manifest is the subset of a Slack app manifest LocalSlack reads; other
// fields are stored verbatim by the registry.
type Manifest struct {
	DisplayInformation DisplayInformation `json:"display_information"`
	Features           Features           `json:"features"`
	OAuthConfig        OAuthConfig        `json:"oauth_config"`
	Settings           Settings           `json:"settings"`
}

// DisplayInformation names the app.
type DisplayInformation struct {
	Name string `json:"name"`
}

// Features describes the app's bot user.
type Features struct {
	BotUser BotUser `json:"bot_user"`
}

// BotUser is the app's bot identity.
type BotUser struct {
	DisplayName string `json:"display_name"`
}

// OAuthConfig lists granted scopes.
type OAuthConfig struct {
	Scopes Scopes `json:"scopes"`
}

// Scopes are the bot-token scopes.
type Scopes struct {
	Bot []string `json:"bot"`
}

// Settings holds delivery configuration.
type Settings struct {
	EventSubscriptions EventSubscriptions `json:"event_subscriptions"`
	SocketModeEnabled  bool               `json:"socket_mode_enabled"`
}

// EventSubscriptions says where and what to deliver over HTTP.
type EventSubscriptions struct {
	RequestURL string   `json:"request_url"`
	BotEvents  []string `json:"bot_events"`
}
