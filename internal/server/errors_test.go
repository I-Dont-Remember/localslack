package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// errShape is what a client can tell about an error response.
type errShape struct {
	Status int
	Shape  string // "admin" = {"error":{"code","message"}}, "slack" = {"ok":false,"error":code}
	Code   string
}

func shapeOf(t *testing.T, w *httptest.ResponseRecorder) errShape {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", w.Body.String(), err)
	}
	got := errShape{Status: w.Code}
	switch e := m["error"].(type) {
	case map[string]any:
		got.Shape, got.Code = "admin", e["code"].(string)
		if msg, _ := e["message"].(string); msg == "" {
			t.Errorf("admin error without message: %v", m)
		}
	case string:
		got.Shape, got.Code = "slack", e
		if m["ok"] != false {
			t.Errorf("slack error without ok:false: %v", m)
		}
	}
	return got
}

func TestServerErrorsArePathAware(t *testing.T) {
	t.Parallel()

	const loopback = "127.0.0.1:3232"
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	panicking := recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	router := NewRouter(Deps{Log: log, ListenAddr: loopback})
	big := strings.Repeat("x", maxBodyBytes+1)

	tests := []struct {
		name    string
		h       http.Handler
		method  string
		path    string
		host    string
		headers map[string]string
		body    string
		want    errShape
	}{
		{"bad host, admin", router, "GET", "/_localslack/health", "evil.example", nil, "", errShape{403, "admin", "invalid_host"}},
		{"bad host, api", router, "GET", "/api/auth.test", "evil.example", nil, "", errShape{403, "slack", "invalid_host"}},
		{"bad origin, admin", router, "POST", "/_localslack/reset", "localhost:3232", map[string]string{"Origin": "http://evil.example"}, "", errShape{403, "admin", "forbidden_origin"}},
		{"bad origin, api", router, "POST", "/api/chat.postMessage", "localhost:3232", map[string]string{"Origin": "http://evil.example"}, "", errShape{403, "slack", "forbidden_origin"}},
		{"unknown admin path", router, "GET", "/_localslack/x", "localhost:3232", nil, "", errShape{404, "admin", "not_found"}},
		{"unknown api method keeps slack shape", router, "POST", "/api/nope.nothing", "localhost:3232", nil, "", errShape{404, "slack", "not_implemented"}},
		{"wrong verb, admin", router, "POST", "/_localslack/health", "localhost:3232", nil, "", errShape{405, "admin", "method_not_allowed"}},
		{"oversized body, admin", router, "POST", "/_localslack/reset", "localhost:3232", nil, big, errShape{413, "admin", "payload_too_large"}},
		{"panic, admin", panicking, "GET", "/_localslack/boom", "localhost:3232", nil, "", errShape{500, "admin", "internal_error"}},
		{"panic, api", panicking, "GET", "/api/boom", "localhost:3232", nil, "", errShape{500, "slack", "internal_error"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			tt.h.ServeHTTP(w, req)

			if diff := cmp.Diff(tt.want, shapeOf(t, w)); diff != "" {
				t.Errorf("error response (-want +got):\n%s\nbody: %s", diff, w.Body.String())
			}
		})
	}
}
