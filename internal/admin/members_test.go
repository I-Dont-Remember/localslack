package admin_test

import (
	"net/http"
	"slices"
	"testing"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestChannelMembersHelper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		verb       string
		channel    func(ch map[string]string) string
		user       func(users map[string]string) string
		wantStatus int
		wantCode   string
		wantMember bool // after the call; only checked on 204
	}{
		{"add bot to public", http.MethodPut, func(c map[string]string) string { return c["pub"] }, func(u map[string]string) string { return u["bot"] }, http.StatusNoContent, "", true},
		{"add bot to private", http.MethodPut, func(c map[string]string) string { return c["priv"] }, func(u map[string]string) string { return u["bot"] }, http.StatusNoContent, "", true},
		{"add bot to dm", http.MethodPut, func(c map[string]string) string { return c["dm"] }, func(u map[string]string) string { return u["bot"] }, http.StatusNoContent, "", true},
		{"add again is idempotent", http.MethodPut, func(c map[string]string) string { return c["pub"] }, func(u map[string]string) string { return u["owner"] }, http.StatusNoContent, "", true},
		{"remove member", http.MethodDelete, func(c map[string]string) string { return c["pub"] }, func(u map[string]string) string { return u["owner"] }, http.StatusNoContent, "", false},
		{"remove non-member is idempotent", http.MethodDelete, func(c map[string]string) string { return c["pub"] }, func(u map[string]string) string { return u["bot"] }, http.StatusNoContent, "", false},
		{"unknown channel", http.MethodPut, func(map[string]string) string { return "CNOPE" }, func(u map[string]string) string { return u["bot"] }, http.StatusNotFound, "not_found", false},
		{"unknown user", http.MethodPut, func(c map[string]string) string { return c["pub"] }, func(map[string]string) string { return "UNOPE" }, http.StatusNotFound, "not_found", false},
		{"unknown channel on delete", http.MethodDelete, func(map[string]string) string { return "CNOPE" }, func(u map[string]string) string { return u["bot"] }, http.StatusNotFound, "not_found", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)
			chans, users := membersEnv(t, app)

			got := rawDo(t, app, tt.verb, "/_localslack/channels/"+tt.channel(chans)+"/members/"+tt.user(users), "")

			if got.Status != tt.wantStatus {
				t.Fatalf("status = %d (%s), want %d", got.Status, got.Body, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusNoContent {
				if code := got.apiError(t).Error.Code; code != tt.wantCode {
					t.Errorf("error code = %q, want %q", code, tt.wantCode)
				}
				return
			}
			page, err := app.Service.Members(t.Context(), service.User{ID: users["owner"]}, tt.channel(chans), service.PageOpts{Limit: 1000})
			if err != nil {
				t.Fatalf("Members: %v", err)
			}
			if has := slices.Contains(page.Items, tt.user(users)); has != tt.wantMember {
				t.Errorf("member = %v, want %v (members %v)", has, tt.wantMember, page.Items)
			}
		})
	}
}

// membersEnv: the owner is in pub, priv and dm; the bot is in none.
func membersEnv(t *testing.T, app *storetest.App) (chans, users map[string]string) {
	t.Helper()
	ctx := t.Context()
	all, err := app.Service.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	users = map[string]string{}
	var owner, other service.User
	for _, u := range all {
		switch {
		case u.IsBot && users["bot"] == "":
			users["bot"] = u.ID
		case !u.IsBot && owner.ID == "":
			owner = u
		case !u.IsBot && other.ID == "":
			other = u
		}
	}
	users["owner"] = owner.ID
	chans = map[string]string{}
	for name, private := range map[string]bool{"pub": false, "priv": true} {
		var opts []service.CreateOpt
		if private {
			opts = append(opts, service.CreatePrivate())
		}
		c, err := app.Service.CreateChannel(ctx, owner, "mem-"+name, opts...)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		chans[name] = c.ID
	}
	dm, err := app.Service.OpenDM(ctx, owner, []string{other.ID})
	if err != nil {
		t.Fatalf("OpenDM: %v", err)
	}
	chans["dm"] = dm.ID
	return chans, users
}
