package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"localslack/internal/server"
)

func TestGuard(t *testing.T) {
	t.Parallel()

	const loopback, open = "127.0.0.1:3232", "0.0.0.0:3232"
	tests := []struct {
		name   string
		listen string
		method string
		host   string
		hdr    map[string]string
		want   int
	}{
		{"localhost host ok", loopback, http.MethodGet, "localhost:3232", nil, http.StatusOK},
		{"127.0.0.1 host ok", loopback, http.MethodGet, "127.0.0.1:3232", nil, http.StatusOK},
		{"ipv6 loopback host ok", loopback, http.MethodGet, "[::1]:3232", nil, http.StatusOK},
		{"rebinding host refused", loopback, http.MethodGet, "evil.example:3232", nil, http.StatusForbidden},
		{"open bind allows any host", open, http.MethodGet, "compose-service:3232", nil, http.StatusOK},
		{"no listen addr, no host rule", "", http.MethodGet, "anything", nil, http.StatusOK},
		{"cross-origin post refused", loopback, http.MethodPost, "localhost:3232", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"other localhost port post refused", loopback, http.MethodPost, "localhost:3232", map[string]string{"Origin": "http://localhost:8000"}, http.StatusForbidden},
		{"same-origin post passes guard", loopback, http.MethodPost, "localhost:3232", map[string]string{"Origin": "http://localhost:3232"}, http.StatusMethodNotAllowed},
		{"sec-fetch-site cross-site refused", loopback, http.MethodPost, "localhost:3232", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"sdk post without origin passes guard", loopback, http.MethodPost, "localhost:3232", nil, http.StatusMethodNotAllowed},
		{"cross-origin post refused on open bind too", open, http.MethodPost, "compose-service:3232", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"cross-origin GET is not a write", loopback, http.MethodGet, "localhost:3232", map[string]string{"Origin": "http://evil.example"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := server.NewRouter(server.Deps{Log: discardLogger(), ListenAddr: tt.listen})
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/_localslack/health", nil)
			req.Host = tt.host
			for k, v := range tt.hdr {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", w.Code, tt.want, strings.TrimSpace(w.Body.String()))
			}
		})
	}
}
