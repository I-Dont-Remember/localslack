package appmanifest_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/appmanifest"
)

func TestDecodeSlackManifest(t *testing.T) {
	t.Parallel()

	const doc = `{
	  "display_information": {"name": "Demo"},
	  "features": {"bot_user": {"display_name": "demo-bot", "always_online": true}},
	  "oauth_config": {"scopes": {"bot": ["chat:write", "app_mentions:read"]}},
	  "settings": {
	    "event_subscriptions": {"request_url": "http://127.0.0.1:9/events", "bot_events": ["app_mention"]},
	    "socket_mode_enabled": false
	  }
	}`
	var got appmanifest.Manifest
	if err := json.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := appmanifest.Manifest{
		DisplayInformation: appmanifest.DisplayInformation{Name: "Demo"},
		Features:           appmanifest.Features{BotUser: appmanifest.BotUser{DisplayName: "demo-bot"}},
		OAuthConfig:        appmanifest.OAuthConfig{Scopes: appmanifest.Scopes{Bot: []string{"chat:write", "app_mentions:read"}}},
		Settings: appmanifest.Settings{EventSubscriptions: appmanifest.EventSubscriptions{
			RequestURL: "http://127.0.0.1:9/events", BotEvents: []string{"app_mention"},
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("manifest (-want +got):\n%s", diff)
	}
}
