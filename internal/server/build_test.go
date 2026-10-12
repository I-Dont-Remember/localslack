package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"localslack/internal/calllog"
	"localslack/internal/config"
	"localslack/internal/events"
	"localslack/internal/server"
	"localslack/internal/service"
)

func TestBuildWiresServiceAndStore(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	cfg := config.Config{Seed: "5", DBPath: filepath.Join(t.TempDir(), "t.db")}
	app, err := server.Build(t.Context(), discardLogger(), cfg, now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if app.Store == nil || app.Service == nil {
		t.Fatalf("Build left Store=%v Service=%v, want both set", app.Store, app.Service)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_localslack/health", nil)
	w := httptest.NewRecorder()
	app.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}
	_, _ = io.Copy(io.Discard, w.Result().Body)
	_ = w.Result().Body.Close()

	if err := app.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

type spyDispatcher struct {
	events.Noop
	mu     sync.Mutex
	got    []service.Event
	closed bool
}

func (s *spyDispatcher) Publish(e service.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, e)
}

func (s *spyDispatcher) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func apiPost(t *testing.T, h http.Handler, method string, form url.Values) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/"+method, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer xoxb-"+"test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	_, _ = io.Copy(io.Discard, w.Result().Body)
	_ = w.Result().Body.Close()
}

func TestBuildWiresComponents(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	cfg := config.Config{Seed: "5", DBPath: filepath.Join(t.TempDir(), "t.db"), CallLogSize: 3}
	spy := &spyDispatcher{}
	app, err := server.Build(t.Context(), discardLogger(), cfg, now, server.WithDispatcher(spy))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	c := app.Components
	if c.CallLog == nil || c.Faults == nil || c.Control == nil || c.Events == nil {
		t.Fatalf("components not wired: %+v", c)
	}

	apiPost(t, app.Handler, "auth.test", nil)
	if got := c.CallLog.Query(t.Context(), calllog.Query{Method: "auth.test"}).Calls; len(got) != 1 {
		t.Errorf("recorded auth.test calls = %d, want 1", len(got))
	}

	general := ""
	convs, _ := app.Service.Snapshot(t.Context())
	for _, cv := range convs {
		if cv.Name == "general" {
			general = cv.ID
		}
	}
	apiPost(t, app.Handler, "chat.postMessage", url.Values{"channel": {general}, "text": {"hi"}})
	spy.mu.Lock()
	n := len(spy.got)
	spy.mu.Unlock()
	if n != 1 {
		t.Errorf("dispatcher received %d events, want 1 MessagePosted", n)
	}

	if err := app.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !spy.closed {
		t.Error("App.Close did not close the dispatcher")
	}
}
