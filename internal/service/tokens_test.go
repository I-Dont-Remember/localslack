package service_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

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
