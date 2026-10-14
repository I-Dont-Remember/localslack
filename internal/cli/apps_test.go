package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type seenReq struct {
	Method, Path, ContentType string
	Body                      string
}

// appsServer fakes the apps endpoints and records requests.
func appsServer(t *testing.T, apps []map[string]any) (*httptest.Server, *[]seenReq) {
	t.Helper()
	var got []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, seenReq{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(b)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/_localslack/apps":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": apps})
		case r.Method == http.MethodPost || r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(apps[0])
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"app_not_found","message":"no such app"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func sampleApp() map[string]any {
	return map[string]any{
		"id": "A1", "name": "Demo", "bot_user_id": "U1", "delivery": "ready", "verified": true, "deliver_to": "http://localhost:3000",
		"credentials":         map[string]any{"signing_secret": "sig", "bot_token": "bt", "app_token": "at", "verification_token": "vt"},
		"event_subscriptions": map[string]any{"effective_request_url": "http://localhost:3000/slack/events"},
		"warnings":            []any{"heads up"},
	}
}

func addrOf(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "http://") }

func TestAppsEnv(t *testing.T) {
	t.Parallel()

	srv, _ := appsServer(t, []map[string]any{sampleApp()})
	for _, ref := range []string{"A1", "demo"} {
		var out bytes.Buffer
		cmd := AppsEnvCmd{Conn: Conn{Addr: addrOf(srv)}, App: ref}
		if err := cmd.run(t.Context(), &out); err != nil {
			t.Fatalf("env %s: %v", ref, err)
		}
		want := "export SLACK_API_URL=" + srv.URL + "/api/\n" +
			"export SLACK_APP_ID=A1\nexport SLACK_BOT_TOKEN=bt\nexport SLACK_SIGNING_SECRET=sig\nexport SLACK_APP_TOKEN=at\n"
		if diff := cmp.Diff(want, out.String()); diff != "" {
			t.Errorf("env %s output (-want +got):\n%s", ref, diff)
		}
	}
	var out bytes.Buffer
	err := (&AppsEnvCmd{Conn: Conn{Addr: addrOf(srv)}, App: "nope"}).run(t.Context(), &out)
	if err == nil || !strings.Contains(err.Error(), "no app") {
		t.Errorf("env unknown err = %v", err)
	}
}

func TestAppsList(t *testing.T) {
	t.Parallel()

	srv, _ := appsServer(t, []map[string]any{sampleApp()})
	var out bytes.Buffer
	if err := (&AppsListCmd{Conn: Conn{Addr: addrOf(srv)}}).run(t.Context(), &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"A1", "Demo", "ready", "http://localhost:3000/slack/events"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestAppsSetURL(t *testing.T) {
	t.Parallel()

	srv, got := appsServer(t, []map[string]any{sampleApp()})
	var out bytes.Buffer
	cmd := AppsSetURLCmd{Conn: Conn{Addr: addrOf(srv)}, App: "demo", URL: "http://localhost:4000"}
	if err := cmd.run(t.Context(), &out); err != nil {
		t.Fatalf("set-url: %v", err)
	}
	last := (*got)[len(*got)-1]
	if last.Method != http.MethodPatch || last.Path != "/_localslack/apps/A1" || last.Body != `{"deliver_to":"http://localhost:4000"}` {
		t.Errorf("request = %+v", last)
	}
	if !strings.Contains(out.String(), "http://localhost:3000/slack/events") {
		t.Errorf("output = %q, want the effective URL", out.String())
	}
}

func TestAppsAdd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(file, []byte("display_information:\n  name: Demo\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	tests := []struct {
		name     string
		cmd      AppsAddCmd
		wantType string
		wantBody string
	}{
		{"raw yaml", AppsAddCmd{}, "application/yaml", "display_information:\n  name: Demo\n"},
		{
			"with deliver-to",
			AppsAddCmd{DeliverTo: "http://localhost:5000"},
			"application/json",
			`{"deliver_to":"http://localhost:5000","manifest":"display_information:\n  name: Demo\n"}`,
		},
		{
			"no verify",
			AppsAddCmd{NoVerify: true},
			"application/json",
			`{"manifest":"display_information:\n  name: Demo\n","verify":false}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, got := appsServer(t, []map[string]any{sampleApp()})
			tt.cmd.Conn = Conn{Addr: addrOf(srv)}
			tt.cmd.Manifest = file
			var out bytes.Buffer
			if err := tt.cmd.run(t.Context(), &out); err != nil {
				t.Fatalf("add: %v", err)
			}
			req := (*got)[0]
			if req.Method != http.MethodPost || req.Path != "/_localslack/apps" || req.ContentType != tt.wantType || req.Body != tt.wantBody {
				t.Errorf("request = %+v, want type %s body %q", req, tt.wantType, tt.wantBody)
			}
			if !strings.Contains(out.String(), "A1") || !strings.Contains(out.String(), "heads up") {
				t.Errorf("output = %q, want id and warnings", out.String())
			}
		})
	}
}

func TestAppsServerError(t *testing.T) {
	t.Parallel()

	srv, _ := appsServer(t, nil)
	err := (&AppsListCmd{Conn: Conn{Addr: addrOf(srv)}}).run(t.Context(), io.Discard)
	if err != nil {
		t.Fatalf("list on empty: %v", err)
	}
	cmd := AppsSetURLCmd{Conn: Conn{Addr: addrOf(srv)}, App: "A404", URL: "http://x"}
	if err := cmd.run(t.Context(), io.Discard); err == nil || !strings.Contains(err.Error(), "no app") {
		t.Errorf("set-url unknown err = %v", err)
	}
}
