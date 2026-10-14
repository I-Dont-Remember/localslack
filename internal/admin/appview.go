package admin

import (
	"localslack/internal/appmanifest"
	"localslack/internal/events"
	"localslack/internal/service"
)

// appView is the app resource of docs/TESTING_API.md.
type appView struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	BotUserID          string              `json:"bot_user_id"`
	BotID              string              `json:"bot_id"`
	Credentials        credentialsView     `json:"credentials"`
	EventSubscriptions subscriptionsView   `json:"event_subscriptions"`
	DeliverTo          string              `json:"deliver_to"`
	Delivery           string              `json:"delivery"`
	BlockedReason      string              `json:"blocked_reason,omitempty"`
	SocketModeEnabled  bool                `json:"socket_mode_enabled"`
	Scopes             map[string][]string `json:"scopes"`
	Verified           bool                `json:"verified"`
	Warnings           []string            `json:"warnings"`
}

type credentialsView struct {
	SigningSecret     string `json:"signing_secret"`
	BotToken          string `json:"bot_token"`
	AppToken          string `json:"app_token"`
	VerificationToken string `json:"verification_token"`
}

type subscriptionsView struct {
	ManifestRequestURL  string   `json:"manifest_request_url"`
	EffectiveRequestURL string   `json:"effective_request_url"`
	BotEvents           []string `json:"bot_events"`
}

// newAppView renders app. ctl (may be nil) resolves where events go.
func newAppView(app service.App, ctl events.Controller) appView {
	v := appView{
		ID: app.ID, Name: app.Name, BotUserID: app.BotUserID, BotID: app.BotID,
		Credentials: credentialsView{
			SigningSecret: app.SigningSecret, BotToken: app.BotToken, AppToken: app.AppToken, VerificationToken: app.VerificationToken,
		},
		EventSubscriptions: subscriptionsView{ManifestRequestURL: app.RequestURL, BotEvents: nonNil(app.BotEvents)},
		DeliverTo:          app.DeliverTo, Delivery: "ready", SocketModeEnabled: app.SocketMode,
		Scopes: map[string][]string{"bot": nonNil(app.Scopes)}, Verified: app.Verified, Warnings: []string{},
	}
	if p, err := appmanifest.Parse(app.Manifest); err == nil {
		v.Warnings = append(v.Warnings, p.Warnings...)
	}
	if ctl == nil || app.SocketMode {
		return v
	}
	if t := ctl.Resolve(app); t.Blocked {
		v.Delivery, v.BlockedReason = "blocked", t.Reason
		v.Warnings = append(v.Warnings, "delivery blocked: "+t.Reason+"; fix: localslack apps set-url "+app.ID+" http://localhost:3000")
	} else {
		v.EventSubscriptions.EffectiveRequestURL = t.URL
	}
	if msg := ctl.VerifyError(app.ID); msg != "" {
		v.Warnings = append(v.Warnings, "url_verification failed: "+msg)
	}
	return v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
