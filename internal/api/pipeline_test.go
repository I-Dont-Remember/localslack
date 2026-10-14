package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"localslack/internal/api"
	"localslack/internal/calllog"
	"localslack/internal/constants"
	"localslack/internal/faults"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

type sink struct {
	mu   sync.Mutex
	recs []calllog.CallRecord
}

func (s *sink) Record(r calllog.CallRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, r)
}

func (s *sink) all() []calllog.CallRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]calllog.CallRecord(nil), s.recs...)
}

type override struct {
	ov  faults.Override
	ok  bool
	got chan faults.Request
}

func (o *override) Intercept(_ context.Context, r faults.Request) (faults.Override, bool) {
	o.got <- r
	return o.ov, o.ok
}

func pipeline(t *testing.T, opts ...api.Option) *httptest.Server {
	t.Helper()
	svc, err := service.New(t.Context(), storetest.NewStore(t), storetest.PinnedNow, "pipe")
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	r := chi.NewRouter()
	api.Mount(r, svc, slog.New(slog.NewTextHandler(io.Discard, nil)), opts...)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

type reply struct {
	status int
	header http.Header
}

func post(t *testing.T, srv *httptest.Server, method, token string, form url.Values) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/"+method, strings.NewReader(form.Encode()))
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
	_ = resp.Body.Close() // bodies are asserted via the Recorder
	return reply{resp.StatusCode, resp.Header}
}

func TestRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		token  string
		form   url.Values
		want   calllog.CallRecord
	}{
		{
			"success", "auth.test", "xoxb-x", nil,
			calllog.CallRecord{Method: "auth.test", HTTPMethod: "POST", Status: 200, OK: true, Actor: calllog.Actor{TokenKind: "bot"}},
		},
		{
			"error", "auth.test", "hunter2",
			url.Values{"a": {"b"}},
			calllog.CallRecord{
				Method: "auth.test", HTTPMethod: "POST", Status: 200, Error: "invalid_auth",
				Params: map[string]any{"a": "b"}, Actor: calllog.Actor{TokenKind: "invalid"},
			},
		},
		{
			"unknown method", "nope.nothing", "", nil,
			calllog.CallRecord{Method: "nope.nothing", HTTPMethod: "POST", Status: 200, Error: "unknown_method", NotImplemented: true, Actor: calllog.Actor{TokenKind: "none"}},
		},
		{
			"unknown method keeps actor and params", "nope.nothing", "xoxb-x",
			url.Values{"channel": {"C1"}},
			calllog.CallRecord{
				Method: "nope.nothing", HTTPMethod: "POST", Status: 200, Error: "unknown_method", NotImplemented: true,
				Params: map[string]any{"channel": "C1"}, Actor: calllog.Actor{TokenKind: "bot"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &sink{}
			srv := pipeline(t, api.WithRecorder(s))
			post(t, srv, tt.method, tt.token, tt.form)

			recs := s.all()
			if len(recs) != 1 {
				t.Fatalf("records = %d, want 1 (recorded before the response is sent)", len(recs))
			}
			got := recs[0]
			ignore := cmpopts.IgnoreFields(calllog.CallRecord{}, "Time", "RequestID", "DurationMS", "Response", "Actor.UserID")
			if diff := cmp.Diff(tt.want, got, ignore, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("record mismatch (-want +got):\n%s", diff)
			}
			if got.Response["ok"] != tt.want.OK {
				t.Errorf("response ok = %v, want %v", got.Response["ok"], tt.want.OK)
			}
		})
	}
}

func TestInterceptorOverride(t *testing.T) {
	t.Parallel()

	s := &sink{}
	icpt := &override{
		ov:  faults.Override{FaultID: "F1", Respond: true, Status: 429, Headers: http.Header{"Retry-After": {"7"}}, Body: map[string]any{"ok": false, "error": "ratelimited"}},
		ok:  true,
		got: make(chan faults.Request, 1),
	}
	srv := pipeline(t, api.WithRecorder(s), api.WithInterceptor(icpt))
	// bad token, yet still faulted and logged
	resp := post(t, srv, "chat.postMessage", "hunter2", url.Values{"text": {"hi"}})

	req := <-icpt.got
	if want := (faults.Request{Method: "chat.postMessage", HTTPMethod: "POST", Params: url.Values{"text": {"hi"}}, TokenKind: "invalid"}); cmp.Diff(want, req) != "" {
		t.Errorf("intercepted request = %+v, want %+v", req, want)
	}
	if resp.status != 429 || resp.header.Get("Retry-After") != "7" {
		t.Errorf("status %d retry-after %q, want 429 / 7", resp.status, resp.header.Get("Retry-After"))
	}
	if recs := s.all(); len(recs) != 1 || recs[0].FaultID != "F1" || recs[0].Error != "ratelimited" || recs[0].Status != 429 {
		t.Errorf("records = %+v, want one faulted ratelimited record", recs)
	}
}

func TestRecorderFlagsOversizedResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		size          int
		wantTruncated bool
	}{
		{"under cap", 1 << 10, false},
		{"over cap", calllog.MaxResponseBytes + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &sink{}
			icpt := &override{
				ov:  faults.Override{Respond: true, Status: 200, Body: map[string]any{"ok": true, "blob": strings.Repeat("x", tt.size)}},
				ok:  true,
				got: make(chan faults.Request, 1),
			}
			srv := pipeline(t, api.WithRecorder(s), api.WithInterceptor(icpt))
			post(t, srv, "auth.test", "xoxb-x", nil)

			got := s.all()[0]
			if got.ResponseTruncated != tt.wantTruncated {
				t.Errorf("ResponseTruncated = %v, want %v", got.ResponseTruncated, tt.wantTruncated)
			}
			if gotNil := got.Response == nil; gotNil != tt.wantTruncated {
				t.Errorf("Response nil = %v, want %v", gotNil, tt.wantTruncated)
			}
		})
	}
}

func TestInterceptorDelayRunsMethod(t *testing.T) {
	t.Parallel()

	icpt := &override{ov: faults.Override{FaultID: "F2", Delay: time.Millisecond}, ok: true, got: make(chan faults.Request, 1)}
	srv := pipeline(t, api.WithInterceptor(icpt))
	resp := post(t, srv, "auth.test", "xoxb-x", nil)
	if resp.status != 200 {
		t.Errorf("status = %d, want 200 (method ran after delay)", resp.status)
	}
}
