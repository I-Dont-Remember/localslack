package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/config"
	"localslack/internal/respond"
	"localslack/internal/server"
)

const contractDoc = "docs/TESTING_API.md"

// plannedRoutes are documented in docs/TESTING_API.md but not merged yet. Key is
// "METHOD /path" with params as {}; value is the owning stream. Delete the entry
// in the commit that registers the route (a stale entry fails the test).
func plannedRoutes() map[string]string {
	return map[string]string{
		"GET /_localslack/state":                 "G (state)",
		"POST /_localslack/snapshots":            "G (snapshots)",
		"GET /_localslack/snapshots":             "G (snapshots)",
		"GET /_localslack/snapshots/{}":          "G (snapshots)",
		"POST /_localslack/snapshots/{}/restore": "G (snapshots)",
		"GET /_localslack/diff":                  "G (diff)",
		"GET /_localslack/socket/connections":    "S (socket mode)",
		"POST /_localslack/socket/disconnect":    "S (socket mode)",
	}
}

var (
	paramRE = regexp.MustCompile(`\{[^}]*\}`)
	docRE   = regexp.MustCompile(`((?:GET|POST|PUT|PATCH|DELETE)(?:\|(?:GET|POST|PUT|PATCH|DELETE))*) (/_localslack/[A-Za-z0-9_{}/-]+)`)
)

// route is one registered or documented endpoint.
type route struct{ method, pattern string }

func (r route) key() string { return r.method + " " + paramRE.ReplaceAllString(r.pattern, "{}") }

func contractHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Config{Addr: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "contract.db"), Seed: "1"}
	app, err := server.Build(t.Context(), discardLogger(), cfg, time.Now)
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app.Handler
}

func registeredRoutes(t *testing.T, h http.Handler) []route {
	t.Helper()
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("handler %T is not a chi.Routes; cannot enumerate routes", h)
	}
	var out []route
	err := chi.Walk(routes, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(pattern, respond.AdminPrefix) {
			out = append(out, route{method, pattern})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	return out
}

func documentedRoutes(t *testing.T) []route {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", contractDoc))
	if err != nil {
		t.Fatalf("read %s: %v", contractDoc, err)
	}
	var out []route
	for _, m := range docRE.FindAllStringSubmatch(string(raw), -1) {
		for _, method := range strings.Split(m[1], "|") {
			out = append(out, route{method, m[2]})
		}
	}
	return out
}

func keys(rs []route) map[string]bool {
	m := map[string]bool{}
	for _, r := range rs {
		m[r.key()] = true
	}
	return m
}

func TestAdminRoutesMatchDocs(t *testing.T) {
	t.Parallel()

	registered := keys(registeredRoutes(t, contractHandler(t)))
	documented := keys(documentedRoutes(t))

	for _, k := range sortedKeys(registered) {
		if !documented[k] {
			t.Errorf("route %q is registered but not documented: add a line with `%s` to %s (every /_localslack route is part of the contract)", k, k, contractDoc)
		}
		if stream, planned := plannedRoutes()[k]; planned {
			t.Errorf("route %q is registered but still listed in plannedRoutes() (stream %s): remove the entry in route_contract_test.go", k, stream)
		}
	}
	for _, k := range sortedKeys(documented) {
		if _, planned := plannedRoutes()[k]; !registered[k] && !planned {
			t.Errorf("route %q is documented in %s but not registered: implement it, or add it to plannedRoutes in route_contract_test.go with its stream name", k, contractDoc)
		}
	}
	for _, k := range sortedKeys(plannedKeys()) {
		if !documented[k] {
			t.Errorf("plannedRoutes entry %q is not documented in %s: document it or drop the entry", k, contractDoc)
		}
	}
}

func plannedKeys() map[string]bool {
	m := map[string]bool{}
	for k := range plannedRoutes() {
		m[k] = true
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// do sends one in-process request as a loopback client.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:0"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// checkAdminError verifies the /_localslack error contract on w.
func checkAdminError(t *testing.T, what string, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code < 400 || w.Code > 499 {
		t.Errorf("%s: status %d, want 4xx: /_localslack uses real HTTP statuses, never 200 + ok:false (see %s)", what, w.Code, contractDoc)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Errorf("%s: body %q is not a JSON object: errors must be {\"error\":{\"code\",\"message\"}} (see %s)", what, w.Body.String(), contractDoc)
		return
	}
	if _, has := m["ok"]; has {
		t.Errorf("%s: error body has an `ok` key: /_localslack has no ok wrapper, only {\"error\":{\"code\",\"message\"}} (see %s)", what, contractDoc)
	}
	var e struct{ Code, Message string }
	if err := json.Unmarshal(m["error"], &e); err != nil || e.Code == "" || e.Message == "" {
		t.Errorf("%s: body %s must be {\"error\":{\"code\":\"snake_case\",\"message\":\"...\"}} (see %s)", what, w.Body.String(), contractDoc)
	}
}

func TestAdminRouteErrorsFollowContract(t *testing.T) {
	t.Parallel()

	h := contractHandler(t)
	paths := map[string]map[string]bool{}
	for _, r := range registeredRoutes(t, h) {
		if paths[r.pattern] == nil {
			paths[r.pattern] = map[string]bool{}
		}
		paths[r.pattern][r.method] = true
		concrete := paramRE.ReplaceAllString(r.pattern, "nonexistent")
		body := ""
		if r.method == http.MethodPost || r.method == http.MethodPut || r.method == http.MethodPatch {
			body = "{not json"
			if concrete != r.pattern {
				body = "{}" // unknown id must be a 4xx whatever the body
			}
		}
		if body == "" && concrete == r.pattern {
			continue // nothing to provoke on a bodyless collection route; wrong-verb below covers its path
		}
		checkAdminError(t, r.method+" "+concrete+" (provoked: unknown id / malformed body)", do(h, r.method, concrete, body))
	}
	for pattern, have := range paths {
		for _, v := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if have[v] {
				continue
			}
			w := do(h, v, paramRE.ReplaceAllString(pattern, "nonexistent"), "")
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: status %d, want 405 method_not_allowed (see %s)", v, pattern, w.Code, contractDoc)
			}
			checkAdminError(t, v+" "+pattern+" (wrong verb)", w)
			break
		}
	}
}

// probe is a request expected to succeed; $name in path/body expands to a value
// captured by an earlier probe (capture: name -> top-level JSON path "a.b.0.c").
type probe struct {
	route   string // "METHOD /pattern" as registered
	path    string // concrete path; defaults to the route path with params left as-is
	body    string
	capture map[string]string
}

// successProbes must cover every registered /_localslack route, in an order that works.
// New route? Add a probe here (the test tells you which one is missing).
func successProbes() []probe {
	return []probe{
		{route: "GET /_localslack/health"},
		{route: "POST /_localslack/apps", body: `{"manifest":{"display_information":{"name":"contract probe"},"settings":{"event_subscriptions":{"bot_events":["message.channels"]}}},"deliver_to":"http://127.0.0.1:1"}`, capture: map[string]string{"app": "id"}},
		{route: "GET /_localslack/apps"},
		{route: "GET /_localslack/apps/{id}", path: "/_localslack/apps/$app"},
		{route: "PATCH /_localslack/apps/{id}", path: "/_localslack/apps/$app", body: `{"deliver_to":"http://127.0.0.1:1"}`},
		{route: "POST /_localslack/apps/{id}/verify", path: "/_localslack/apps/$app/verify"},
		{route: "GET /_localslack/apps/{id}/delivery-config", path: "/_localslack/apps/$app/delivery-config"},
		{route: "PUT /_localslack/apps/{id}/delivery-config", path: "/_localslack/apps/$app/delivery-config", body: `{"paused":true}`},
		{route: "PATCH /_localslack/apps/{id}/delivery-config", path: "/_localslack/apps/$app/delivery-config", body: `{"paused":false}`},
		{route: "DELETE /_localslack/apps/{id}/delivery-config", path: "/_localslack/apps/$app/delivery-config"},
		{route: "GET /_localslack/delivery-config"},
		{route: "PUT /_localslack/delivery-config", body: `{}`},
		{route: "PATCH /_localslack/delivery-config", body: `{"paused":true}`},
		{route: "POST /_localslack/events", body: `{"event":{"type":"message","text":"x"}}`, capture: map[string]string{"event": "deliveries.0.event_id", "delivery": "deliveries.0.delivery_id"}},
		{route: "GET /_localslack/deliveries"},
		{route: "GET /_localslack/deliveries/{id}", path: "/_localslack/deliveries/$delivery"},
		{route: "POST /_localslack/events/{event_id}/redeliver", path: "/_localslack/events/$event/redeliver", body: `{}`},
		{route: "POST /_localslack/deliveries/{id}/redeliver", path: "/_localslack/deliveries/$delivery/redeliver", body: `{}`},
		{route: "DELETE /_localslack/deliveries"},
		{route: "POST /_localslack/faults", body: `{"method":"chat.postMessage","action":{"status":429,"error":"ratelimited"}}`, capture: map[string]string{"fault": "id"}},
		{route: "GET /_localslack/faults"},
		{route: "DELETE /_localslack/faults/{id}", path: "/_localslack/faults/$fault"},
		{route: "DELETE /_localslack/faults"},
		{route: "GET /_localslack/calls"},
		{route: "DELETE /_localslack/calls"},
		{route: "GET /_localslack/state/conversations"},
		{route: "POST /_localslack/reset", body: `{}`},
		{route: "DELETE /_localslack/apps/{id}", path: "/_localslack/apps/$app"},
	}
}

func jsonPath(v any, path string) string {
	for _, part := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[part]
		case []any:
			if part == "0" && len(x) > 0 {
				v = x[0]
			} else {
				return ""
			}
		default:
			return ""
		}
	}
	s, _ := v.(string)
	return s
}

func TestAdminRouteSuccessFollowsContract(t *testing.T) {
	t.Parallel()

	h := contractHandler(t)
	covered := map[string]bool{}
	vars := map[string]string{}
	expand := func(s string) string {
		for k, v := range vars {
			s = strings.ReplaceAll(s, "$"+k, v)
		}
		return s
	}
	for _, p := range successProbes() {
		covered[paramRE.ReplaceAllString(p.route, "{}")] = true
		method, pattern, _ := strings.Cut(p.route, " ")
		path := p.path
		if path == "" {
			path = pattern
		}
		w := do(h, method, expand(path), expand(p.body))
		what := p.route + " (success probe)"
		if w.Code < 200 || w.Code > 299 {
			t.Errorf("%s: status %d body %s, want 2xx: fix the probe or the handler (see %s)", what, w.Code, w.Body.String(), contractDoc)
			continue
		}
		raw, _ := io.ReadAll(w.Body)
		if w.Code == http.StatusNoContent {
			if len(raw) != 0 {
				t.Errorf("%s: 204 must have an empty body, got %q (see %s)", what, raw, contractDoc)
			}
			continue
		}
		var body any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("%s: body %q is not JSON (see %s)", what, raw, contractDoc)
			continue
		}
		if m, isObj := body.(map[string]any); isObj {
			if _, has := m["ok"]; has {
				t.Errorf("%s: success body has a top-level `ok` key: /_localslack returns the resource itself with a real 2xx status, no ok wrapper (see %s)", what, contractDoc)
			}
		}
		for name, jp := range p.capture {
			if vars[name] = jsonPath(body, jp); vars[name] == "" {
				t.Errorf("%s: capture %q found nothing at %q in %s", what, name, jp, raw)
			}
		}
	}
	for _, r := range registeredRoutes(t, h) {
		if !covered[r.key()] {
			t.Errorf("route %q has no entry in successProbes (route_contract_test.go): add one so its success body is checked against %s", r.key(), contractDoc)
		}
	}
}
