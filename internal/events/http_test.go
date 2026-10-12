package events_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/events"
)

// seen is what a fake app recorded about one request.
type seen struct {
	Method, Path, Body string
	Header             http.Header
}

type recorder struct {
	mu   sync.Mutex
	reqs []seen
}

func (r *recorder) add(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, seen{Method: req.Method, Path: req.URL.Path, Body: string(body), Header: req.Header.Clone()})
}

func (r *recorder) all() []seen {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seen(nil), r.reqs...)
}

func fixedClock() events.Clock { return events.NewManualClock(time.Unix(1_700_000_000, 0)) }

func TestHTTPTransportDeliver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ctxTimeout time.Duration
		spec       events.Spec
		want       events.Attempt // compared on the fields below only
		wantOK     bool
	}{
		{
			name:    "2xx acks",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") },
			spec:    events.Spec{Secret: "s", Mode: events.SigValid},
			want:    events.Attempt{Status: 200, ResponseBody: "ok", SignatureMode: "valid"},
			wantOK:  true,
		},
		{
			name:    "non-2xx fails as http_error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			spec:    events.Spec{Secret: "s", Mode: events.SigInvalid},
			want:    events.Attempt{Status: 401, Error: "unexpected HTTP status 401", FailReason: "http_error", SignatureMode: "invalid"},
		},
		{
			name: "redirect is not followed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
			},
			spec: events.Spec{Secret: "s", Mode: events.SigValid},
			want: events.Attempt{Status: 302, Error: "redirect not followed (HTTP 302)", FailReason: "too_many_redirects", SignatureMode: "valid"},
		},
		{
			name: "no-retry header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Slack-No-Retry", "1")
				w.WriteHeader(http.StatusInternalServerError)
			},
			spec: events.Spec{Secret: "s", Mode: events.SigValid},
			want: events.Attempt{Status: 500, Error: "unexpected HTTP status 500", FailReason: "http_error", NoRetry: true, SignatureMode: "valid"},
		},
		{
			name:       "timeout",
			handler:    func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
			ctxTimeout: 50 * time.Millisecond,
			spec:       events.Spec{Secret: "s", Mode: events.SigValid},
			want:       events.Attempt{Error: "timeout", FailReason: "http_timeout", SignatureMode: "valid"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rec.add(r)
				tt.handler(w, r)
			}))
			t.Cleanup(srv.Close)
			tt.spec.URL = srv.URL + "/slack/events"

			ctx := t.Context()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			tr := events.NewHTTPTransport(fixedClock())
			got := tr.Deliver(ctx, events.Delivery{Payload: []byte(`{"x":1}`), Spec: tt.spec})

			if got.OK() != tt.wantOK {
				t.Errorf("OK() = %v, want %v", got.OK(), tt.wantOK)
			}
			slim := events.Attempt{
				Status: got.Status, Error: got.Error, ResponseBody: got.ResponseBody, SignatureMode: got.SignatureMode,
				NoRetry: got.NoRetry, FailReason: got.FailReason,
			}
			if diff := cmp.Diff(tt.want, slim); diff != "" {
				t.Errorf("attempt (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHTTPTransportRequestShape(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rec.add(r) }))
	t.Cleanup(srv.Close)

	body := []byte(`{"type":"event_callback"}`)
	tr := events.NewHTTPTransport(fixedClock())
	got := tr.Deliver(t.Context(), events.Delivery{
		Payload: body,
		Spec:    events.Spec{URL: srv.URL + "/hooks/x", Secret: "sek", Mode: events.SigValid, RetryNum: 2, RetryReason: "http_timeout"},
	})

	reqs := rec.all()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/hooks/x" || r.Body != string(body) {
		t.Errorf("request = %s %s %q", r.Method, r.Path, r.Body)
	}
	wantSig := events.Sign("sek", "1700000000", body)
	want := map[string]string{
		"Content-Type": constants.ContentTypeJSON, "X-Slack-Request-Timestamp": "1700000000", "X-Slack-Signature": wantSig,
		"X-Slack-Retry-Num": "2", "X-Slack-Retry-Reason": "http_timeout",
	}
	for k, v := range want {
		if g := r.Header.Get(k); g != v {
			t.Errorf("header %s = %q, want %q", k, g, v)
		}
	}
	if got.Timestamp != "1700000000" || got.Signature != wantSig {
		t.Errorf("attempt ts/sig = %q/%q, want %q/%q", got.Timestamp, got.Signature, "1700000000", wantSig)
	}
}

func TestHTTPTransportMissingSignatureOmitsHeaders(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rec.add(r) }))
	t.Cleanup(srv.Close)

	tr := events.NewHTTPTransport(fixedClock())
	tr.Deliver(t.Context(), events.Delivery{Payload: []byte(`{}`), Spec: events.Spec{URL: srv.URL, Secret: "s", Mode: events.SigMissing}})

	h := rec.all()[0].Header
	if h.Get("X-Slack-Signature") != "" || h.Get("X-Slack-Request-Timestamp") != "" {
		t.Errorf("headers = %v, want no signature or timestamp", h)
	}
	if h.Get("X-Slack-Retry-Num") != "" {
		t.Errorf("first attempt carries retry header %q", h.Get("X-Slack-Retry-Num"))
	}
}

func TestHTTPTransportConnectionRefused(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	got := events.NewHTTPTransport(fixedClock()).Deliver(t.Context(), events.Delivery{Spec: events.Spec{URL: url, Secret: "s"}})
	if got.OK() || got.FailReason != "connection_failed" || got.Error == "" {
		t.Errorf("attempt = %+v, want connection_failed", got)
	}
}
