package service_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/appmanifest"
	"localslack/internal/service"
)

func parsed(t *testing.T, name, requestURL string, events ...string) appmanifest.Parsed {
	t.Helper()
	m := map[string]any{
		"display_information": map[string]any{"name": name},
		"features":            map[string]any{"bot_user": map[string]any{"display_name": name + " Bot"}},
		"oauth_config":        map[string]any{"scopes": map[string]any{"bot": []string{"chat:write"}}},
		"settings": map[string]any{"event_subscriptions": map[string]any{
			"request_url": requestURL, "bot_events": events,
		}},
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

func TestRegisterAppCreatesBotUserAndCredentials(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	app, err := svc.RegisterApp(ctx, service.RegisterInput{
		Manifest: parsed(t, "Demo App", "http://127.0.0.1:3000/slack/events", "app_mention", "message.channels"),
	})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}

	if !strings.HasPrefix(app.ID, "A") || len(app.ID) != 11 {
		t.Errorf("app id = %q, want A + 10 chars", app.ID)
	}
	if !strings.HasPrefix(app.BotUserID, "U") || !strings.HasPrefix(app.BotID, "B") {
		t.Errorf("bot ids = %q / %q", app.BotUserID, app.BotID)
	}
	for name, v := range map[string]string{
		"signing secret": app.SigningSecret, "bot token": app.BotToken, "app token": app.AppToken, "verification token": app.VerificationToken,
	} {
		if v == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if !strings.HasPrefix(app.BotToken, "xo"+"xb-") || !strings.HasPrefix(app.AppToken, "xa"+"pp-") {
		t.Errorf("token prefixes: %q %q", app.BotToken, app.AppToken)
	}
	if app.Name != "Demo App" || app.RequestURL != "http://127.0.0.1:3000/slack/events" || app.SocketMode {
		t.Errorf("app = %+v", app)
	}
	if diff := cmp.Diff([]string{"app_mention", "message.channels"}, app.BotEvents); diff != "" {
		t.Errorf("bot events (-want +got):\n%s", diff)
	}

	p, err := svc.Authenticate(ctx, app.BotToken)
	if err != nil {
		t.Fatalf("Authenticate(bot token): %v", err)
	}
	if p.App == nil || p.App.ID != app.ID || p.User.ID != app.BotUserID || !p.User.IsBot || p.User.BotID != app.BotID {
		t.Errorf("principal = %+v, want the new app acting as its own bot user", p)
	}
	if p.User.Name != "demo-app-bot" || p.User.RealName != "Demo App Bot" {
		t.Errorf("bot user name = %q / %q", p.User.Name, p.User.RealName)
	}
	if p, err := svc.Authenticate(ctx, app.AppToken); err != nil || p.App == nil || p.App.ID != app.ID {
		t.Errorf("Authenticate(app token) = %+v, %v", p, err)
	}
}

func TestRegisterAppJoinsBotToPublicChannels(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	ctx := t.Context()
	app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Joiner", "", "message.channels")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	convs, err := st.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	var ids []string
	for _, c := range convs {
		if c.Kind == service.KindChannel && !c.IsPrivate && !c.IsArchived {
			ids = append(ids, c.ID)
		}
	}
	members, err := st.MembersOf(ctx, ids)
	if err != nil {
		t.Fatalf("MembersOf: %v", err)
	}
	for _, id := range ids {
		if !slices.Contains(members[id], app.BotUserID) {
			t.Errorf("bot is not a member of public channel %s", id)
		}
	}
}

func TestRegisterAppDeterministicAcrossServers(t *testing.T) {
	t.Parallel()

	reg := func() service.App {
		svc, _ := newSvc(t)
		app, err := svc.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed(t, "Same Name", "")})
		if err != nil {
			t.Fatalf("RegisterApp: %v", err)
		}
		return app
	}
	a, b := reg(), reg()
	a.Created, b.Created = 0, 0
	if diff := cmp.Diff(a, b); diff != "" {
		t.Errorf("same manifest name on two servers differs (-a +b):\n%s", diff)
	}
	other, _ := newSvc(t)
	c, err := other.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed(t, "Different", "")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if c.ID == a.ID || c.BotToken == a.BotToken || c.SigningSecret == a.SigningSecret || c.BotUserID == a.BotUserID {
		t.Errorf("distinct names collided: %+v vs %+v", c, a)
	}
}

func TestRegisterAppOverrides(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	app, err := svc.RegisterApp(t.Context(), service.RegisterInput{
		Manifest: parsed(t, "Custom", ""), AppID: "A0CUSTOM01", SigningSecret: "my-secret",
		BotToken: "xo" + "xb-custom", AppToken: "xa" + "pp-custom", DeliverTo: "http://localhost:4000",
	})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if app.ID != "A0CUSTOM01" || app.SigningSecret != "my-secret" || app.BotToken != "xo"+"xb-custom" ||
		app.AppToken != "xa"+"pp-custom" || app.DeliverTo != "http://localhost:4000" {
		t.Errorf("overrides not applied: %+v", app)
	}
	if p, err := svc.Authenticate(t.Context(), "xo"+"xb-custom"); err != nil || p.App == nil || p.App.ID != "A0CUSTOM01" {
		t.Errorf("custom bot token does not resolve: %+v, %v", p, err)
	}
}

func TestRegisterAppDuplicateAndUpsert(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	first, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Twice", "http://127.0.0.1:1/a", "app_mention"), DeliverTo: "http://localhost:9"})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	_, err = svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Twice", "")})
	if !errors.Is(err, service.ErrAppExists) {
		t.Fatalf("second RegisterApp err = %v, want ErrAppExists", err)
	}

	again, err := svc.RegisterApp(ctx, service.RegisterInput{
		Manifest: parsed(t, "Twice", "http://127.0.0.1:2/b", "message.channels"), Upsert: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if again.ID != first.ID || again.BotUserID != first.BotUserID || again.BotToken != first.BotToken || again.SigningSecret != first.SigningSecret {
		t.Errorf("upsert changed identity/credentials: %+v vs %+v", again, first)
	}
	if again.DeliverTo != "http://localhost:9" {
		t.Errorf("upsert clobbered stored deliver_to: %q", again.DeliverTo)
	}
	if again.RequestURL != "http://127.0.0.1:2/b" || !slices.Equal(again.BotEvents, []string{"message.channels"}) {
		t.Errorf("upsert did not refresh manifest fields: %+v", again)
	}
	if n := len(svc.Apps()); n != 1 {
		t.Errorf("apps = %d, want 1", n)
	}
}

func TestDeleteAppRevokesTokens(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Gone", "")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if err := svc.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if p := svc.Identify(ctx, app.BotToken); p.App != nil {
		t.Errorf("deleted app still resolves: %+v", p.App)
	}
	if err := svc.DeleteApp(ctx, app.ID); !errors.Is(err, service.ErrAppNotFound) {
		t.Errorf("second DeleteApp err = %v, want ErrAppNotFound", err)
	}
	if _, err := svc.App(ctx, app.ID); !errors.Is(err, service.ErrAppNotFound) {
		t.Errorf("App(deleted) err = %v, want ErrAppNotFound", err)
	}
	// The bot user and its history stay, as a deactivated bot would.
	users, err := svc.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if !slices.ContainsFunc(users, func(u service.User) bool { return u.ID == app.BotUserID }) {
		t.Error("bot user was deleted with the app")
	}
}

func TestSetAppDeliverToAndVerified(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Tweak", "")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	updated, err := svc.SetAppDeliverTo(ctx, app.ID, "http://localhost:5000")
	if err != nil || updated.DeliverTo != "http://localhost:5000" {
		t.Fatalf("SetAppDeliverTo = %+v, %v", updated, err)
	}
	if err := svc.SetAppVerified(ctx, app.ID, true); err != nil {
		t.Fatalf("SetAppVerified: %v", err)
	}
	got, err := svc.App(ctx, app.ID)
	if err != nil || !got.Verified || got.DeliverTo != "http://localhost:5000" {
		t.Errorf("App = %+v, %v; want verified and deliver_to persisted", got, err)
	}
	if _, err := svc.SetAppDeliverTo(ctx, "A404", "x"); !errors.Is(err, service.ErrAppNotFound) {
		t.Errorf("SetAppDeliverTo(unknown) err = %v", err)
	}
	if err := svc.SetAppVerified(ctx, "A404", true); !errors.Is(err, service.ErrAppNotFound) {
		t.Errorf("SetAppVerified(unknown) err = %v", err)
	}
}

func TestAppsSortedByID(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if _, err := svc.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed(t, n, "")}); err != nil {
			t.Fatalf("RegisterApp(%s): %v", n, err)
		}
	}
	apps := svc.Apps()
	if len(apps) != 3 || !slices.IsSortedFunc(apps, func(a, b service.App) int { return strings.Compare(a.ID, b.ID) }) {
		t.Errorf("Apps() = %+v, want 3 sorted by id", apps)
	}
}
