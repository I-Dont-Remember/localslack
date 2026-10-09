package server

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const maxBodyBytes = 2 << 20

// isLoopbackAddr reports whether a listen address only accepts local connections.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

// guard defends a tool whose auth is "any token works":
//   - on a loopback bind, only local Host names are served (blocks DNS rebinding);
//   - state-changing requests from another origin are refused (blocks CSRF, which
//     would otherwise reach /api/* with a token in a plain form body);
//   - request bodies are capped.
//
// listenAddr "" disables the Host rule (unit tests); origin and body rules always apply.
func guard(log *slog.Logger, listenAddr string, next http.Handler) http.Handler {
	var allowedHosts map[string]bool
	if isLoopbackAddr(listenAddr) {
		allowedHosts = map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, hostOnly(listenAddr): true}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedHosts != nil && !allowedHosts[hostOnly(r.Host)] {
			deny(log, w, r, "invalid_host")
			return
		}
		if !isSafeMethod(r.Method) && crossOrigin(r) {
			deny(log, w, r, "forbidden_origin")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// crossOrigin is true when a browser says the request came from elsewhere.
// Non-browser clients (the Slack SDKs, curl) send neither header.
func crossOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	return err != nil || u.Host != r.Host
}

func deny(log *slog.Logger, w http.ResponseWriter, r *http.Request, code string) {
	writeJSON(r.Context(), log, w, http.StatusForbidden, errorBody(code))
}
