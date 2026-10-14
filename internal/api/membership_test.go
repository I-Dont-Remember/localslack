package api_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

// membershipEnv: a human owns a public, a private and an archived channel; the default bot is in none.
type membershipEnv struct {
	app                *storetest.App
	human, bot         string // user ids
	humanTok           string
	pub, priv, archive string
}

func newMembershipEnv(t *testing.T) *membershipEnv {
	t.Helper()
	app := storetest.NewApp(t)
	e := &membershipEnv{app: app, human: firstHuman(t, app)["id"].(string)}
	e.humanTok = "xoxp-" + e.human
	_, auth := call(t, app, "auth.test", botToken, nil)
	e.bot, _ = auth["user_id"].(string)
	mk := func(name string, private bool) string {
		p := map[string]string{"name": name}
		if private {
			p["is_private"] = "true"
		}
		_, b := call(t, app, "conversations.create", e.humanTok, p)
		return b["channel"].(map[string]any)["id"].(string)
	}
	e.pub, e.priv, e.archive = mk("m-pub", false), mk("m-priv", true), mk("m-arch", false)
	call(t, app, "conversations.archive", e.humanTok, map[string]string{"channel": e.archive})
	return e
}

// pick projects the keys a membership test asserts on.
func pick(body map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := body[k]; ok {
			out[k] = v
		}
	}
	return out
}

func TestConversationsJoin(t *testing.T) {
	t.Parallel()
	warn := map[string]any{"ok": true, "warning": "already_in_channel", "response_metadata": map[string]any{"warnings": []any{"already_in_channel"}}}
	tests := []struct {
		name  string
		token string
		ch    func(*membershipEnv) string
		want  map[string]any
	}{
		{"bot joins public", botToken, func(e *membershipEnv) string { return e.pub }, map[string]any{"ok": true}},
		{"owner already in", "", func(e *membershipEnv) string { return e.pub }, warn},
		{"private", botToken, func(e *membershipEnv) string { return e.priv }, map[string]any{"ok": false, "error": "channel_not_found"}},
		{"archived", botToken, func(e *membershipEnv) string { return e.archive }, map[string]any{"ok": false, "error": "is_archived"}},
		{"unknown", botToken, func(*membershipEnv) string { return "CNOPE" }, map[string]any{"ok": false, "error": "channel_not_found"}},
		{"missing channel", botToken, func(*membershipEnv) string { return "" }, map[string]any{"ok": false, "error": "channel_not_found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newMembershipEnv(t)
			tok := tt.token
			if tok == "" {
				tok = e.humanTok
			}

			_, body := call(t, e.app, "conversations.join", tok, map[string]string{"channel": tt.ch(e)})

			if diff := cmp.Diff(tt.want, pick(body, "ok", "error", "warning", "response_metadata")); diff != "" {
				t.Errorf("response (-want +got):\n%s", diff)
			}
			if ch, ok := body["channel"].(map[string]any); body["ok"] == true && (!ok || ch["is_member"] != true || ch["id"] != tt.ch(e)) {
				t.Errorf("channel = %v, want the joined channel with is_member", body["channel"])
			}
		})
	}
}

func TestJoinThenLeaveChangesAccess(t *testing.T) {
	t.Parallel()
	e := newMembershipEnv(t)
	hist := func() string {
		_, b := call(t, e.app, "conversations.history", botToken, map[string]string{"channel": e.pub})
		if b["ok"] == true {
			return ""
		}
		return b["error"].(string)
	}
	steps := []struct {
		name, method, wantResult, wantHistory string
	}{
		{"leave before join", "conversations.leave", "not_in_channel", "not_in_channel"},
		{"join", "conversations.join", "", ""},
		{"leave", "conversations.leave", "", "not_in_channel"},
	}
	for _, s := range steps {
		_, b := call(t, e.app, s.method, botToken, map[string]string{"channel": e.pub})
		got, _ := b["error"].(string)
		if got != s.wantResult || hist() != s.wantHistory {
			t.Errorf("%s: error %q history %q, want %q / %q", s.name, got, hist(), s.wantResult, s.wantHistory)
		}
	}
}

func TestConversationsLeaveErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token string
		ch    func(*membershipEnv) string
		want  string
	}{
		{"private non-member", botToken, func(e *membershipEnv) string { return e.priv }, "channel_not_found"},
		{"general", "", func(e *membershipEnv) string { return generalID(t, e.app) }, "cant_leave_general"},
		{"archived member", "", func(e *membershipEnv) string { return e.archive }, "is_archived"},
		{"unknown", botToken, func(*membershipEnv) string { return "CNOPE" }, "channel_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newMembershipEnv(t)
			tok := tt.token
			if tok == "" {
				tok = e.humanTok
			}
			_, body := call(t, e.app, "conversations.leave", tok, map[string]string{"channel": tt.ch(e)})
			if diff := cmp.Diff(map[string]any{"ok": false, "error": tt.want}, pick(body, "ok", "error")); diff != "" {
				t.Errorf("response (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConversationsInvite(t *testing.T) {
	t.Parallel()
	fail := func(code string, users ...string) map[string]any {
		errs := []any{}
		for _, u := range users {
			errs = append(errs, map[string]any{"ok": false, "error": code, "user": u})
		}
		return map[string]any{"ok": false, "error": code, "errors": errs}
	}
	tests := []struct {
		name        string
		ch          func(*membershipEnv) string
		users       func(*membershipEnv) string
		force       bool
		inviteFirst bool
		want        func(*membershipEnv) map[string]any
	}{
		{
			"bot into private", func(e *membershipEnv) string { return e.priv }, func(e *membershipEnv) string { return e.bot }, false, false,
			func(*membershipEnv) map[string]any { return map[string]any{"ok": true} },
		},
		{
			"already in channel", func(e *membershipEnv) string { return e.pub }, func(e *membershipEnv) string { return e.bot }, false, true,
			func(e *membershipEnv) map[string]any { return fail("already_in_channel", e.bot) },
		},
		{
			"unknown user", func(e *membershipEnv) string { return e.pub }, func(*membershipEnv) string { return "UNOPE" }, false, false,
			func(*membershipEnv) map[string]any { return fail("user_not_found", "UNOPE") },
		},
		{
			"self", func(e *membershipEnv) string { return e.pub }, func(e *membershipEnv) string { return e.human }, false, false,
			func(e *membershipEnv) map[string]any { return fail("cant_invite_self", e.human) },
		},
		{
			"no users", func(e *membershipEnv) string { return e.pub }, func(*membershipEnv) string { return "" }, false, false,
			func(*membershipEnv) map[string]any {
				return map[string]any{"ok": false, "error": "users_list_not_supplied"}
			},
		},
		{
			"archived", func(e *membershipEnv) string { return e.archive }, func(e *membershipEnv) string { return e.bot }, false, false,
			func(*membershipEnv) map[string]any { return map[string]any{"ok": false, "error": "is_archived"} },
		},
		{
			"unknown channel", func(*membershipEnv) string { return "CNOPE" }, func(e *membershipEnv) string { return e.bot }, false, false,
			func(*membershipEnv) map[string]any { return map[string]any{"ok": false, "error": "channel_not_found"} },
		},
		{
			"one bad user fails the batch", func(e *membershipEnv) string { return e.pub }, func(e *membershipEnv) string { return e.bot + ",UNOPE" }, false, false,
			func(*membershipEnv) map[string]any { return fail("user_not_found", "UNOPE") },
		},
		{
			"force keeps the good ones", func(e *membershipEnv) string { return e.pub }, func(e *membershipEnv) string { return e.bot + ",UNOPE" }, true, false,
			func(*membershipEnv) map[string]any { return map[string]any{"ok": true} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newMembershipEnv(t)
			ch := tt.ch(e)
			if tt.inviteFirst {
				call(t, e.app, "conversations.invite", e.humanTok, map[string]string{"channel": ch, "users": e.bot})
			}
			params := map[string]string{"channel": ch, "users": tt.users(e)}
			if tt.force {
				params["force"] = "true"
			}

			_, body := call(t, e.app, "conversations.invite", e.humanTok, params)

			if diff := cmp.Diff(tt.want(e), pick(body, "ok", "error", "errors")); diff != "" {
				t.Errorf("response (-want +got):\n%s", diff)
			}
			if ch, ok := body["channel"].(map[string]any); body["ok"] == true && (!ok || ch["id"] == nil) {
				t.Errorf("channel = %v, want the conversation", body["channel"])
			}
		})
	}
}

func TestInviteMakesBotAMember(t *testing.T) {
	t.Parallel()
	e := newMembershipEnv(t)
	call(t, e.app, "conversations.invite", e.humanTok, map[string]string{"channel": e.priv, "users": e.bot})

	_, hist := call(t, e.app, "chat.postMessage", botToken, map[string]string{"channel": e.priv, "text": "hi"})

	if hist["ok"] != true {
		t.Errorf("bot post to the private channel it was invited to = %v, want ok", hist)
	}
}

func TestConversationsKick(t *testing.T) {
	t.Parallel()
	e := newMembershipEnv(t)
	call(t, e.app, "conversations.invite", e.humanTok, map[string]string{"channel": e.priv, "users": e.bot})
	tests := []struct {
		name, user, want string
	}{
		{"kick bot", e.bot, ""},
		{"kick again", e.bot, "not_in_channel"},
		{"kick self", e.human, "cant_kick_self"},
		{"unknown user", "UNOPE", "user_not_found"},
	}
	for _, tt := range tests { // sequential: each step depends on the one before
		_, body := call(t, e.app, "conversations.kick", e.humanTok, map[string]string{"channel": e.priv, "user": tt.user})
		got, _ := body["error"].(string)
		if got != tt.want {
			t.Errorf("%s: error = %q, want %q", tt.name, got, tt.want)
		}
	}
	_, b := call(t, e.app, "conversations.history", botToken, map[string]string{"channel": e.priv})
	if b["error"] != "channel_not_found" {
		t.Errorf("kicked bot history = %v, want channel_not_found", b)
	}
}
