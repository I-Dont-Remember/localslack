package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"localslack/internal/appmanifest"
	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

const socketPath = "/_localslack/socket"

// connect registers a socket app, opens a connection through the Slack method and returns it.
func connect(t *testing.T, app *storetest.App, name string) (*websocket.Conn, service.App) {
	t.Helper()
	parsed, err := appmanifest.Parse([]byte(`{"display_information":{"name":"` + name + `"},"settings":{"socket_mode_enabled":true}}`))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	reg, err := app.Service.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return reconnect(t, app, reg), reg
}

func reconnect(t *testing.T, app *storetest.App, reg service.App) *websocket.Conn {
	t.Helper()
	form := url.Values{}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL+"/api/apps.connections.open", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", constants.ContentTypeForm)
	req.Header.Set("Authorization", "Bearer "+reg.AppToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct{ URL string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.URL == "" {
		t.Fatalf("open reply: url %q err %v", out.URL, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c, hs, err := websocket.Dial(ctx, out.URL, nil)
	if hs != nil && hs.Body != nil {
		_ = hs.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial %s: %v", out.URL, err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	readMsg(t, c) // hello
	return c
}

func readMsg(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return m
}

func listConns(t *testing.T, app *storetest.App) []map[string]any {
	t.Helper()
	r, body := adminDo(t, app, http.MethodGet, socketPath+"/connections", "")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, body %v", r.StatusCode, body)
	}
	items, _ := body["items"].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, it.(map[string]any))
	}
	return out
}

func waitConns(t *testing.T, app *storetest.App, want int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := listConns(t, app)
		if len(got) == want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("connections = %d, want %d: %v", len(got), want, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSocketConnectionsList(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	if got := listConns(t, app); len(got) != 0 {
		t.Fatalf("fresh server lists %v, want none", got)
	}
	_, reg := connect(t, app, "Lister")
	got := waitConns(t, app, 1)[0]
	if got["app_id"] != reg.ID || got["id"] == "" || got["pending"] != float64(0) || got["connected_at"] == nil {
		t.Errorf("connection = %v, want app_id %s, pending 0, connected_at set", got, reg.ID)
	}
	if v, present := got["last_ack_at"]; !present || v != nil {
		t.Errorf("last_ack_at = %v (present %v), want explicit null before any ack", v, present)
	}
}

func TestSocketDisconnect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body, wantReason string
		wantMsg                bool
	}{
		{"refresh", `{"reason":"refresh_requested"}`, "refresh_requested", true},
		{"warning", `{"reason":"warning"}`, "warning", true},
		{"link disabled", `{"reason":"link_disabled"}`, "link_disabled", true},
		{"default reason", `{}`, "refresh_requested", true},
		{"drop sends nothing", `{"reason":"warning","drop":true}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)
			c, _ := connect(t, app, "Dropper")
			r, body := adminDo(t, app, http.MethodPost, socketPath+"/disconnect", tt.body)
			if r.StatusCode != http.StatusNoContent || body != nil {
				t.Fatalf("disconnect = %d %v, want 204 empty", r.StatusCode, body)
			}
			if tt.wantMsg {
				m := readMsg(t, c)
				if m["type"] != "disconnect" || m["reason"] != tt.wantReason {
					t.Errorf("message = %v, want disconnect/%s", m, tt.wantReason)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if _, raw, err := c.Read(ctx); err == nil {
				t.Errorf("socket still open, read %s", raw)
			}
			waitConns(t, app, 0)
		})
	}
}

func TestSocketDisconnectByID(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	connect(t, app, "One")
	connect(t, app, "Two")
	conns := waitConns(t, app, 2)
	id, _ := conns[0]["id"].(string)

	r, _ := adminDo(t, app, http.MethodPost, socketPath+"/disconnect", `{"connection_id":"`+id+`","drop":true}`)
	if r.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", r.StatusCode)
	}
	left := waitConns(t, app, 1)
	if left[0]["id"] == id {
		t.Errorf("disconnected connection %s is still listed", id)
	}
}

func TestSocketDisconnectErrors(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	connect(t, app, "Errs")
	tests := []struct {
		name, body, wantCode string
		wantStatus           int
	}{
		{"malformed json", `{nope`, "invalid_json", http.StatusBadRequest},
		{"unknown connection", `{"connection_id":"SCnope","reason":"warning"}`, "connection_not_found", http.StatusNotFound},
		{"bad reason", `{"reason":"because"}`, "invalid_reason", http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, body := adminDo(t, app, http.MethodPost, socketPath+"/disconnect", tt.body)
			e, _ := body["error"].(map[string]any)
			if r.StatusCode != tt.wantStatus || e["code"] != tt.wantCode {
				t.Errorf("got %d %v, want %d %s", r.StatusCode, body, tt.wantStatus, tt.wantCode)
			}
		})
	}
	if n := len(listConns(t, app)); n != 1 {
		t.Errorf("connections = %d after failed disconnects, want 1", n)
	}
}

func TestSocketSurvivesPartialResetButNotRemoval(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	c, reg := connect(t, app, "Survivor")

	if r, body := adminDo(t, app, http.MethodPost, "/_localslack/reset", `{}`); r.StatusCode != http.StatusOK {
		t.Fatalf("reset = %d %v", r.StatusCode, body)
	}
	if got := listConns(t, app); len(got) != 1 {
		t.Fatalf("partial reset left %d connections, want 1", len(got))
	}

	if r, body := adminDo(t, app, http.MethodPost, "/_localslack/reset?all=true", `{}`); r.StatusCode != http.StatusOK {
		t.Fatalf("reset all = %d %v", r.StatusCode, body)
	}
	waitConns(t, app, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Error("socket of an app removed by reset --all is still open")
	}
	_ = reg
}

func TestSocketClosesWhenAppDeleted(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	c, reg := connect(t, app, "Doomed")
	if r, _ := adminDo(t, app, http.MethodDelete, "/_localslack/apps/"+reg.ID, ""); r.StatusCode != http.StatusNoContent {
		t.Fatalf("delete app = %d", r.StatusCode)
	}
	waitConns(t, app, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Error("socket still open after its app was deleted")
	}
}
