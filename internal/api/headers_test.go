package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

func headersOf(t *testing.T, app *storetest.App, method, token string) http.Header {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL+"/api/"+method, strings.NewReader(url.Values{}.Encode()))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeForm)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()
	return resp.Header
}

func TestSlackResponseHeaders(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	tests := []struct {
		name, method, token string
		want                map[string]string // header -> value; "" means absent
	}{
		{"bot success", "auth.test", "xoxb-test", map[string]string{
			"Content-Type":            "application/json; charset=utf-8",
			"X-Oauth-Scopes":          "chat:write,chat:write.public",
			"X-Accepted-Oauth-Scopes": "",
		}},
		{"scoped method", "users.list", "xoxb-test", map[string]string{
			"X-Accepted-Oauth-Scopes": "users:read",
			"X-Oauth-Scopes":          "chat:write,chat:write.public",
		}},
		{"invalid auth has no scopes", "auth.test", "bogus", map[string]string{
			"Content-Type":   "application/json; charset=utf-8",
			"X-Oauth-Scopes": "",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := headersOf(t, app, tt.method, tt.token)
			if h.Get("X-Slack-Req-Id") == "" || h.Get("X-Slack-Req-Id") != h.Get("X-Request-Id") {
				t.Errorf("x-slack-req-id = %q, want non-empty equal to X-Request-Id %q", h.Get("X-Slack-Req-Id"), h.Get("X-Request-Id"))
			}
			for k, want := range tt.want {
				if got := h.Get(k); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
		})
	}
}
