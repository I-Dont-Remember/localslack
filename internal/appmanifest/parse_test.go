package appmanifest_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/appmanifest"
)

func TestParse(t *testing.T) {
	t.Parallel()

	const yamlDoc = `
display_information:
  name: Yaml App
features:
  bot_user:
    display_name: yaml-bot
oauth_config:
  scopes:
    bot: [chat:write]
settings:
  socket_mode_enabled: true
  event_subscriptions:
    bot_events: [app_mention, team_join]
`
	tests := []struct {
		name         string
		in           string
		want         appmanifest.Manifest
		wantWarnings []string
	}{
		{
			name: "json",
			in:   `{"display_information":{"name":"Demo"},"features":{"bot_user":{"display_name":"demo-bot"}},"settings":{"event_subscriptions":{"request_url":"http://127.0.0.1:9/e","bot_events":["app_mention"]}}}`,
			want: appmanifest.Manifest{
				DisplayInformation: appmanifest.DisplayInformation{Name: "Demo"},
				Features:           appmanifest.Features{BotUser: appmanifest.BotUser{DisplayName: "demo-bot"}},
				Settings: appmanifest.Settings{EventSubscriptions: appmanifest.EventSubscriptions{
					RequestURL: "http://127.0.0.1:9/e", BotEvents: []string{"app_mention"},
				}},
			},
		},
		{
			name: "yaml with unsupported event",
			in:   yamlDoc,
			want: appmanifest.Manifest{
				DisplayInformation: appmanifest.DisplayInformation{Name: "Yaml App"},
				Features:           appmanifest.Features{BotUser: appmanifest.BotUser{DisplayName: "yaml-bot"}},
				OAuthConfig:        appmanifest.OAuthConfig{Scopes: appmanifest.Scopes{Bot: []string{"chat:write"}}},
				Settings: appmanifest.Settings{
					SocketModeEnabled:  true,
					EventSubscriptions: appmanifest.EventSubscriptions{BotEvents: []string{"app_mention", "team_join"}},
				},
			},
			wantWarnings: []string{`/settings/event_subscriptions/bot_events/1: event "team_join" is not emitted by LocalSlack`},
		},
		{
			name: "bot display name defaults to app name",
			in:   `{"display_information":{"name":"Plain"}}`,
			want: appmanifest.Manifest{
				DisplayInformation: appmanifest.DisplayInformation{Name: "Plain"},
				Features:           appmanifest.Features{BotUser: appmanifest.BotUser{DisplayName: "Plain"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := appmanifest.Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if diff := cmp.Diff(tt.want, got.Manifest); diff != "" {
				t.Errorf("manifest (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantWarnings, got.Warnings); diff != "" {
				t.Errorf("warnings (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseKeepsRawAsJSON(t *testing.T) {
	t.Parallel()

	got, err := appmanifest.Parse([]byte("display_information:\n  name: X\nextra:\n  k: [1, 2]\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := `{"display_information":{"name":"X"},"extra":{"k":[1,2]}}`; string(got.Raw) != want {
		t.Errorf("Raw = %s, want %s", got.Raw, want)
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []appmanifest.Problem
	}{
		{"missing name", `{}`, []appmanifest.Problem{{Pointer: "/display_information/name", Message: "required"}}},
		{
			"bad request url",
			`{"display_information":{"name":"A"},"settings":{"event_subscriptions":{"request_url":"ftp://x/y"}}}`,
			[]appmanifest.Problem{{Pointer: "/settings/event_subscriptions/request_url", Message: "must be an http(s) URL"}},
		},
		{
			"wrong type scopes",
			`{"display_information":{"name":"A"},"oauth_config":{"scopes":{"bot":"chat:write"}}}`,
			[]appmanifest.Problem{{Pointer: "/oauth_config/scopes/bot", Message: "must be an array of strings"}},
		},
		{
			"empty event name and non-bool socket mode",
			`{"display_information":{"name":"A"},"settings":{"socket_mode_enabled":"yes","event_subscriptions":{"bot_events":["app_mention",""]}}}`,
			[]appmanifest.Problem{
				{Pointer: "/settings/socket_mode_enabled", Message: "must be a boolean"},
				{Pointer: "/settings/event_subscriptions/bot_events/1", Message: "must not be empty"},
			},
		},
		{"not an object", `[1,2]`, []appmanifest.Problem{{Pointer: "", Message: "manifest must be an object"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := appmanifest.Parse([]byte(tt.in))
			var ve *appmanifest.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Parse err = %v, want *ValidationError", err)
			}
			if diff := cmp.Diff(tt.want, ve.Problems); diff != "" {
				t.Errorf("problems (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseMalformed(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "{", "a: [b"} {
		if _, err := appmanifest.Parse([]byte(in)); !errors.Is(err, appmanifest.ErrMalformed) {
			t.Errorf("Parse(%q) err = %v, want ErrMalformed", in, err)
		}
	}
}
