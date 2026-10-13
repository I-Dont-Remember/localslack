package admin_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/config"
	"localslack/internal/events"
	"localslack/internal/server"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

type resetReply struct {
	Mode       string `json:"mode"`
	Generation int64  `json:"generation"`
	Seed       string `json:"seed"`
	Counts     struct {
		Users, Channels, Messages int
	} `json:"counts"`
	TookMS int64 `json:"took_ms"`
}

type errReply struct {
	Error struct{ Code, Message string } `json:"error"`
}

func postReset(t *testing.T, base, query, body string) (int, []byte) {
	t.Helper()
	u := base + "/_localslack/reset"
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post reset: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, b
}

func TestResetReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		body     string
		wantMode string
		wantSeed string
	}{
		{"partial default, no body", "", "", "partial", ""},
		{"partial with seed", "", `{"seed":"abc"}`, "partial", "abc"},
		{"all", "all=true", `{"seed":"abc"}`, "all", "abc"},
		{"all=false is partial", "all=false", "{}", "partial", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)

			status, b := postReset(t, app.URL, tt.query, tt.body)

			var got resetReply
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("decode %s: %v", b, err)
			}
			if status != http.StatusOK || got.Mode != tt.wantMode || got.Generation != 1 {
				t.Errorf("reply = %d %+v, want 200, mode %s, generation 1", status, got, tt.wantMode)
			}
			if tt.wantSeed != "" && got.Seed != tt.wantSeed {
				t.Errorf("seed = %q, want %q", got.Seed, tt.wantSeed)
			}
			if got.Seed == "" || got.Counts.Users == 0 || got.Counts.Channels == 0 || got.Counts.Messages == 0 {
				t.Errorf("reply %+v lacks seed or counts", got)
			}
		})
	}
}

func TestResetRejectsBadRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"bad json", "", `{`, http.StatusBadRequest, "invalid_json"},
		{"unknown field", "", `{"keep":["apps"]}`, http.StatusBadRequest, "invalid_reset"},
		{"bad all", "all=maybe", "", http.StatusBadRequest, "invalid_query"},
		{"scenario and generate", "", `{"scenario":"x.json","generate":{"users":3}}`, http.StatusBadRequest, "invalid_reset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)
			before := mustUsers(t, app)

			status, b := postReset(t, app.URL, tt.query, tt.body)

			var e errReply
			_ = json.Unmarshal(b, &e)
			if status != tt.wantStatus || e.Error.Code != tt.wantCode || e.Error.Message == "" {
				t.Errorf("got %d %s, want %d %s with a message", status, b, tt.wantStatus, tt.wantCode)
			}
			if diff := cmp.Diff(before, mustUsers(t, app)); diff != "" {
				t.Errorf("rejected reset changed users (-before +after):\n%s", diff)
			}
		})
	}
}

func mustUsers(t *testing.T, app *storetest.App) []string {
	t.Helper()
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	out := []string{}
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func TestResetSameSeedSameIDs(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	postReset(t, app.URL, "", `{"seed":"X"}`)
	first := mustUsers(t, app)
	postReset(t, app.URL, "", `{"seed":"Y"}`)
	postReset(t, app.URL, "", `{"seed":"X"}`)

	if diff := cmp.Diff(first, mustUsers(t, app)); diff != "" {
		t.Errorf("reset(seed=X) ids differ (-first +again):\n%s", diff)
	}
}

func TestResetClearsCallLogAndKeepsSeqMonotonic(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	slackCall(t, app.URL, "auth.test", nil)
	_, before := listCalls(t, app.URL, "")

	postReset(t, app.URL, "", "")
	_, mid := listCalls(t, app.URL, "")
	slackCall(t, app.URL, "auth.test", nil)
	_, after := listCalls(t, app.URL, "")

	if len(mid.Items) != 0 || mid.Generation <= before.Generation {
		t.Errorf("after reset: %d items, generation %d (was %d), want empty and a new generation", len(mid.Items), mid.Generation, before.Generation)
	}
	if len(after.Items) != 1 || after.Items[0].Seq <= before.Items[0].Seq {
		t.Errorf("seq after reset = %+v, want greater than %d", after.Items, before.Items[0].Seq)
	}
}

func TestResetWakesParkedCallPoll(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	_, base := listCalls(t, app.URL, "")
	done := make(chan callsPage, 1)
	go func() {
		_, page := listCalls(t, app.URL, "min=1&timeout_ms=20000")
		done <- page
	}()
	waitUntil(t, "the long poll to park", func() bool { return app.Components.CallLog.Waiters() > 0 })

	postReset(t, app.URL, "", "")

	select {
	case page := <-done:
		if page.Generation <= base.Generation || len(page.Items) != 0 {
			t.Errorf("woken poll = gen %d items %d, want a newer generation and no items", page.Generation, len(page.Items))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parked poll did not wake on reset")
	}
}

func TestResetWipesWorkspaceViaAPI(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	postReset(t, app.URL, "", `{"seed":"fresh"}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL+"/api/conversations.create", strings.NewReader(url.Values{"name": {"made-before"}}.Encode()))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer xoxb-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = resp.Body.Close()

	postReset(t, app.URL, "", `{"seed":"fresh"}`)

	caller, err := app.Service.DefaultBot(t.Context())
	if err != nil {
		t.Fatalf("DefaultBot: %v", err)
	}
	all, err := app.Service.ListAll(t.Context(), caller.User, nil, false)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	for _, c := range all {
		if c.Name == "made-before" {
			t.Errorf("channel %q survived the reset", c.Name)
		}
	}
}

// blockingDispatcher parks inside Reset so a second reset can race the first.
type blockingDispatcher struct {
	events.Noop
	entered, release chan struct{}
}

func (b blockingDispatcher) Reset() { close(b.entered); <-b.release }

func TestConcurrentResetGets409(t *testing.T) {
	t.Parallel()

	disp := blockingDispatcher{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := config.Config{DBPath: t.TempDir() + "/ls.db", Seed: storetest.PinnedSeed}
	built, err := server.Build(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, storetest.PinnedNow, server.WithDispatcher(disp))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = built.Close() })
	srv := httptest.NewServer(built.Handler)
	t.Cleanup(srv.Close)
	firstDone := make(chan int, 1)
	go func() {
		status, _ := postReset(t, srv.URL, "", "")
		firstDone <- status
	}()
	<-disp.entered

	status, b := postReset(t, srv.URL, "", "")
	close(disp.release)

	var e errReply
	_ = json.Unmarshal(b, &e)
	if status != http.StatusConflict || e.Error.Code != "reset_in_progress" {
		t.Errorf("concurrent reset = %d %s, want 409 reset_in_progress", status, b)
	}
	if got := <-firstDone; got != http.StatusOK {
		t.Errorf("first reset = %d, want 200", got)
	}
}

func TestResetDoesNotEmitEvents(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	var got atomic.Int32
	app.Service.Subscribe(func(service.Event) { got.Add(1) })

	postReset(t, app.URL, "", `{"seed":"quiet"}`)
	postReset(t, app.URL, "all=true", "")

	if n := got.Load(); n != 0 {
		t.Errorf("events during reset = %d, want none", n)
	}
}
