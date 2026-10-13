package service_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/appmanifest"
	"localslack/internal/scenario"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

func newSvcWith(t *testing.T, opts ...service.Option) *service.Service {
	t.Helper()
	svc, err := service.New(t.Context(), storetest.NewStore(t), fixedClock(), "svc-ids", opts...)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func TestAuthenticateUnregisteredBotToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		strict  bool
		wantErr error
	}{
		{"permissive maps to the default bot", false, nil},
		{"strict rejects", true, service.ErrInvalidAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvcWith(t, service.WithStrictTokens(tt.strict))

			p, err := svc.Authenticate(t.Context(), "xoxb-nobody")

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if gotBot := p.User.IsBot; gotBot != (tt.wantErr == nil) {
				t.Errorf("resolved bot user = %v, want %v", gotBot, tt.wantErr == nil)
			}
		})
	}
}

func TestStrictModeStillAcceptsRegisteredAndDefaultBot(t *testing.T) {
	t.Parallel()

	svc := newSvcWith(t, service.WithStrictTokens(true))
	app, err := svc.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed(t, "Strict", "")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}

	if p, err := svc.Authenticate(t.Context(), app.BotToken); err != nil || p.App == nil {
		t.Errorf("registered bot token = %+v, %v", p, err)
	}
	if _, err := svc.Authenticate(t.Context(), "xoxp-"+firstHumanID(t, svc)); err != nil {
		t.Errorf("user token: %v", err)
	}
}

func firstHumanID(t *testing.T, svc *service.Service) string {
	t.Helper()
	caller, _ := usersOf(t, svc)
	return caller.ID
}

func TestDeletedAppTokensAreInvalidInEveryMode(t *testing.T) {
	t.Parallel()

	for _, strict := range []bool{false, true} {
		for _, kind := range []string{"bot", "app"} {
			name := kind + " strict=" + map[bool]string{false: "false", true: "true"}[strict]
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				svc := newSvcWith(t, service.WithStrictTokens(strict))
				ctx := t.Context()
				app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Tomb", "")})
				if err != nil {
					t.Fatalf("RegisterApp: %v", err)
				}
				token := map[string]string{"bot": app.BotToken, "app": app.AppToken}[kind]
				if err := svc.DeleteApp(ctx, app.ID); err != nil {
					t.Fatalf("DeleteApp: %v", err)
				}

				if _, err := svc.Authenticate(ctx, token); !errors.Is(err, service.ErrInvalidAuth) {
					t.Errorf("deleted app %s token err = %v, want ErrInvalidAuth", kind, err)
				}
			})
		}
	}
}

func TestTombstonesClearOnReregisterAndResetAll(t *testing.T) {
	t.Parallel()

	svc := newSvcWith(t)
	ctx := t.Context()
	register := func() service.App {
		app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Phoenix", "")})
		if err != nil {
			t.Fatalf("RegisterApp: %v", err)
		}
		return app
	}
	app := register()
	if err := svc.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}

	again := register() // same id => same deterministic token
	if _, err := svc.Authenticate(ctx, again.BotToken); err != nil {
		t.Errorf("re-registered token rejected: %v", err)
	}
	if err := svc.DeleteApp(ctx, again.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if _, err := svc.Authenticate(ctx, again.BotToken); !errors.Is(err, service.ErrInvalidAuth) {
		t.Fatalf("before reset --all err = %v, want ErrInvalidAuth", err)
	}

	if _, err := svc.Reset(ctx, service.ResetOpts{All: true, Seed: "x"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := svc.Authenticate(ctx, again.BotToken); err != nil {
		t.Errorf("after reset --all err = %v, want tombstones cleared (default bot)", err)
	}
}

func TestRegisterAppRejectsDuplicateTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   service.RegisterInput
	}{
		{"bot token", service.RegisterInput{BotToken: "xoxb-shared"}},
		{"app token", service.RegisterInput{AppToken: "xapp-shared"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newSvc(t)
			ctx := t.Context()
			first := tt.in
			first.Manifest = parsed(t, "First", "")
			if _, err := svc.RegisterApp(ctx, first); err != nil {
				t.Fatalf("first RegisterApp: %v", err)
			}
			second := tt.in
			second.Manifest = parsed(t, "Second", "")

			_, err := svc.RegisterApp(ctx, second)

			if !errors.Is(err, service.ErrTokenInUse) {
				t.Errorf("err = %v, want ErrTokenInUse", err)
			}
			if n := len(svc.Apps()); n != 1 {
				t.Errorf("apps = %d, want 1 (nothing stored)", n)
			}
			upsert := first
			upsert.Upsert = true
			if _, err := svc.RegisterApp(ctx, upsert); err != nil {
				t.Errorf("upserting the owner with its own token: %v", err)
			}
		})
	}
}

func TestDefaultBotReportsDefaultScopes(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)

	p, err := svc.Authenticate(t.Context(), "xoxb-whatever")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if diff := cmp.Diff(service.DefaultBotScopes(), p.Scopes); diff != "" {
		t.Errorf("default bot scopes (-want +got):\n%s", diff)
	}
	for _, want := range []string{"chat:write", "chat:write.public"} {
		found := false
		for _, s := range p.Scopes {
			found = found || s == want
		}
		if !found {
			t.Errorf("default bot scopes %v lack %s", p.Scopes, want)
		}
	}
}

// The unregistered xoxb- token is the built-in bot, never an app's bot user, even
// when that user sorts first (live app) or lingers after its app was deleted.
func TestUnregisteredBotTokenIsTheBuiltInBotNotAnAppBot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		deleteApp bool
	}{
		{"registered app", false},
		{"deleted app leaves its bot user", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newSvc(t)
			ctx := t.Context()
			app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Scoped", "")}) // scopes: chat:write only
			if err != nil {
				t.Fatalf("RegisterApp: %v", err)
			}
			builtIn := app.BotUserID + "Z" // sorts after the app's bot user
			_, err = svc.Restore(ctx, scenario.Fixture{
				Users: []scenario.User{
					{ID: "UHUMAN1", Name: "ann"},
					{ID: builtIn, Name: "builtin", IsBot: true, BotID: "BBUILTIN"},
				},
				Channels: []scenario.Channel{
					{Name: "general", Members: []string{"UHUMAN1", builtIn}},
					{Name: "open", Members: []string{"UHUMAN1"}},
				},
			})
			if err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if tt.deleteApp {
				if err := svc.DeleteApp(ctx, app.ID); err != nil {
					t.Fatalf("DeleteApp: %v", err)
				}
			}

			p, err := svc.Authenticate(ctx, "xoxb-whatever")

			if err != nil || p.User.ID != builtIn || p.App != nil {
				t.Errorf("xoxb-whatever = user %q app %v err %v, want the built-in bot %q", p.User.ID, p.App, err, builtIn)
			}
		})
	}
}

func parsedWithScopes(t *testing.T, name string, scopes []string) appmanifest.Parsed {
	t.Helper()
	m := map[string]any{
		"display_information": map[string]any{"name": name},
		"features":            map[string]any{"bot_user": map[string]any{"display_name": name}},
	}
	if scopes != nil {
		m["oauth_config"] = map[string]any{"scopes": map[string]any{"bot": scopes}}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	p, err := appmanifest.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}

// Scopes follow the token's app: a manifest without chat:write.public cannot post to an unjoined
// public channel (not_in_channel is correct), the built-in bot can.
func TestPostToUnjoinedPublicChannelFollowsTheTokensScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scopes  []string
		wantErr error
	}{
		{"manifest without chat:write.public", []string{"chat:write"}, service.ErrNotInChannel},
		{"manifest with chat:write.public", []string{"chat:write", "chat:write.public"}, nil},
		{"manifest without scopes gets the defaults", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newSvc(t)
			ctx := t.Context()
			human, _ := usersOf(t, svc)
			open, err := svc.CreateChannel(ctx, human, "open")
			if err != nil {
				t.Fatalf("CreateChannel: %v", err)
			}
			app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsedWithScopes(t, "Scopes", tt.scopes)})
			if err != nil {
				t.Fatalf("RegisterApp: %v", err)
			}
			if err := svc.SetMembership(ctx, open.ID, app.BotUserID, false); err != nil { // registration joined it only to channels that existed
				t.Fatalf("SetMembership: %v", err)
			}
			p, err := svc.Authenticate(ctx, app.BotToken)
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}

			_, err = svc.PostMessage(ctx, p.User, service.PostOpts{Channel: open.ID, Content: service.Content{Text: "hi"}})

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Errorf("post err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
