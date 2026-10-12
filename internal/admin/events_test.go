package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

// recApp records every event_callback it gets (and acks url_verification).
type recApp struct {
	*httptest.Server
	mu   sync.Mutex
	hdrs []http.Header
}

func newRecApp(t *testing.T) *recApp {
	t.Helper()
	a := &recApp{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b struct{ Type, Challenge string }
		_ = json.Unmarshal(raw, &b)
		if b.Type == "url_verification" {
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": b.Challenge})
			return
		}
		a.mu.Lock()
		a.hdrs = append(a.hdrs, r.Header.Clone())
		a.mu.Unlock()
	}))
	t.Cleanup(a.Close)
	return a
}

func register(t *testing.T, base, name, deliverTo string, events ...string) string {
	t.Helper()
	status, _, got := doReq(t, http.MethodPost, base+"/_localslack/apps", constants.ContentTypeJSON, map[string]any{
		"manifest": manifest(name, "", events...), "deliver_to": deliverTo,
	})
	if status != http.StatusCreated {
		t.Fatalf("register %s: %d %v", name, status, got)
	}
	return got["id"].(string)
}

func TestDeliveriesFlowFromUIPostToLongPoll(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newRecApp(t)
	id := register(t, app.URL, "Flow", fake.URL, "app_mention", "message.channels")
	a, err := app.Service.App(t.Context(), id)
	if err != nil {
		t.Fatalf("App: %v", err)
	}
	human, general := humanAndGeneral(t, app.Service)

	if _, err := app.Service.PostMessage(t.Context(), human, service.PostOpts{
		Channel: general, Content: service.Content{Text: "hey <@" + a.BotUserID + "> hello"},
	}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	status, _, got := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=2&timeout_ms=5000", "", nil)
	items, _ := got["items"].([]any)
	if status != http.StatusOK || len(items) != 2 {
		t.Fatalf("deliveries status %d body %v, want message + app_mention delivered", status, got)
	}
	types := map[string]bool{}
	for _, it := range items {
		m := it.(map[string]any)
		types[m["event_type"].(string)] = true
		if m["app_id"] != id || m["transport"] != "http" {
			t.Errorf("delivery = %v", m)
		}
	}
	if !types["message"] || !types["app_mention"] {
		t.Errorf("event types = %v", types)
	}
	if got["generation"] == nil || got["next_since"] == nil {
		t.Errorf("list envelope = %v", got)
	}

	first := items[0].(map[string]any)["id"].(string)
	if status, _, one := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries/"+first, "", nil); status != http.StatusOK || one["id"] != first {
		t.Errorf("get delivery: %d %v", status, one)
	}
	if status, _, _ := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries/nope", "", nil); status != http.StatusNotFound {
		t.Errorf("get unknown delivery status = %d", status)
	}
	if status, _, _ := doReq(t, http.MethodDelete, app.URL+"/_localslack/deliveries", "", nil); status != http.StatusNoContent {
		t.Errorf("delete status = %d", status)
	}
	_, _, after := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries", "", nil)
	if len(after["items"].([]any)) != 0 {
		t.Errorf("deliveries after delete = %v", after["items"])
	}
}

func TestInjectAndRedeliver(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newRecApp(t)
	id := register(t, app.URL, "Inj", fake.URL, "reaction_added")

	status, _, res := doReq(t, http.MethodPost, app.URL+"/_localslack/events", constants.ContentTypeJSON, map[string]any{
		"event": map[string]any{"type": "reaction_added", "reaction": "tada"}, "signature": "invalid",
	})
	if status != http.StatusCreated {
		t.Fatalf("inject status %d %v", status, res)
	}
	ds := res["deliveries"].([]any)
	if _, top := res["event_id"]; top || len(ds) != 1 || ds[0].(map[string]any)["app_id"] != id {
		t.Fatalf("response = %v, want deliveries only (event ids are per app)", res)
	}
	evID, deliveryID := ds[0].(map[string]any)["event_id"].(string), ds[0].(map[string]any)["delivery_id"].(string)
	doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=1&timeout_ms=5000", "", nil)

	status, _, again := doReq(t, http.MethodPost, app.URL+"/_localslack/events/"+evID+"/redeliver", constants.ContentTypeJSON,
		map[string]any{"retry_num": 2, "retry_reason": "http_timeout"})
	if got := again["deliveries"].([]any)[0].(map[string]any); status != http.StatusCreated || got["event_id"] != evID || got["app_id"] != id {
		t.Fatalf("redeliver status %d %v", status, again)
	}
	status, _, byDelivery := doReq(t, http.MethodPost, app.URL+"/_localslack/deliveries/"+deliveryID+"/redeliver", constants.ContentTypeJSON, map[string]any{})
	if got := byDelivery["deliveries"].([]any)[0].(map[string]any); status != http.StatusCreated || got["event_id"] != evID {
		t.Fatalf("redeliver by delivery id status %d %v", status, byDelivery)
	}
	_, _, list := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=3&timeout_ms=5000", "", nil)
	if len(list["items"].([]any)) != 3 {
		t.Fatalf("deliveries = %v", list["items"])
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.hdrs) != 3 || fake.hdrs[1].Get("X-Slack-Retry-Num") != "2" {
		t.Errorf("headers = %v", fake.hdrs)
	}

	for name, tc := range map[string]struct {
		path   string
		body   map[string]any
		status int
		code   string
	}{
		"inject no type":             {"/_localslack/events", map[string]any{"event": map[string]any{}}, 422, "invalid_event"},
		"inject unknown app":         {"/_localslack/events", map[string]any{"event": map[string]any{"type": "x"}, "app_id": "A404"}, 404, "app_not_found"},
		"redeliver unknown":          {"/_localslack/events/Ev404/redeliver", map[string]any{}, 404, "event_not_found"},
		"redeliver wrong app":        {"/_localslack/events/" + evID + "/redeliver", map[string]any{"app_id": "A404"}, 404, "event_not_found"},
		"redeliver unknown delivery": {"/_localslack/deliveries/D404/redeliver", map[string]any{}, 404, "delivery_not_found"},
	} {
		status, _, body := doReq(t, http.MethodPost, app.URL+tc.path, constants.ContentTypeJSON, tc.body)
		if status != tc.status || sub(body, "error")["code"] != tc.code {
			t.Errorf("%s: status %d body %v, want %d %s", name, status, body, tc.status, tc.code)
		}
	}
}

func TestDeliveryConfigPutReplacesPatchMerges(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newRecApp(t)
	id := register(t, app.URL, "Cfg", fake.URL, "message.channels")
	base := app.URL + "/_localslack/delivery-config"
	appBase := app.URL + "/_localslack/apps/" + id + "/delivery-config"
	get := func(url string) map[string]any {
		t.Helper()
		_, _, got := doReq(t, http.MethodGet, url, "", nil)
		return got
	}
	def := get(base)
	if def["signature"] != "valid" || def["paused"] != false || def["timeout_ms"] == nil {
		t.Fatalf("default config %v", def)
	}
	with := func(over map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range def {
			out[k] = v
		}
		for k, v := range over {
			out[k] = v
		}
		return out
	}

	tests := []struct {
		name       string
		method     string
		url        string
		body       map[string]any
		wantStatus int
		want       map[string]any // nil = skip
	}{
		{"global patch merges", http.MethodPatch, base, map[string]any{"signature": "stale"}, 200, with(map[string]any{"signature": "stale"})},
		{"global patch keeps earlier change", http.MethodPatch, base, map[string]any{"stale_seconds": float64(30)}, 200, with(map[string]any{"signature": "stale", "stale_seconds": float64(30)})},
		{"global put resets omitted fields", http.MethodPut, base, map[string]any{"timeout_ms": float64(1234)}, 200, with(map[string]any{"timeout_ms": float64(1234)})},
		{"global put rejects bad value", http.MethodPut, base, map[string]any{"signature": "bogus"}, 422, nil},
		{"global patch rejects bad value", http.MethodPatch, base, map[string]any{"timeout_ms": float64(0)}, 422, nil},
		{
			"app patch is sparse", http.MethodPatch, appBase,
			map[string]any{"signature": "missing"},
			200,
			with(map[string]any{"timeout_ms": float64(1234), "signature": "missing", "override": map[string]any{"signature": "missing"}}),
		},
		{
			"app patch adds to override", http.MethodPatch, appBase,
			map[string]any{"paused": false},
			200,
			with(map[string]any{"timeout_ms": float64(1234), "signature": "missing", "override": map[string]any{"signature": "missing", "paused": false}}),
		},
		{
			"app put replaces override", http.MethodPut, appBase,
			map[string]any{"stale_seconds": float64(5)},
			200,
			with(map[string]any{"timeout_ms": float64(1234), "stale_seconds": float64(5), "override": map[string]any{"stale_seconds": float64(5)}}),
		},
	}
	for _, tt := range tests {
		status, _, got := doReq(t, tt.method, tt.url, constants.ContentTypeJSON, tt.body)
		if status != tt.wantStatus {
			t.Fatalf("%s: status %d body %v, want %d", tt.name, status, got, tt.wantStatus)
		}
		if tt.want == nil {
			if sub(got, "error")["code"] != "invalid_delivery_config" {
				t.Errorf("%s: body %v, want invalid_delivery_config", tt.name, got)
			}
			continue
		}
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", tt.name, diff)
		}
	}

	t.Run("app GET shows effective config plus override and follows global changes", func(t *testing.T) { //nolint:paralleltest // ordered steps share one server
		doReq(t, http.MethodPatch, base, constants.ContentTypeJSON, map[string]any{"timeout_ms": float64(4321)})
		got := get(appBase)
		if got["timeout_ms"] != float64(4321) || got["stale_seconds"] != float64(5) || cmp.Diff(map[string]any{"stale_seconds": float64(5)}, got["override"]) != "" {
			t.Errorf("per-app config %v, want live-inherited timeout and sparse override", got)
		}
		if _, leaked := get(base)["override"]; leaked || get(base)["stale_seconds"] == float64(5) {
			t.Errorf("per-app override leaked into global: %v", get(base))
		}
	})
	t.Run("delete drops the override", func(t *testing.T) { //nolint:paralleltest // ordered steps share one server
		if status, _, _ := doReq(t, http.MethodDelete, appBase, "", nil); status != http.StatusNoContent {
			t.Errorf("delete override status %d", status)
		}
		if got := get(appBase); cmp.Diff(map[string]any{}, got["override"]) != "" {
			t.Errorf("override survived delete: %v", got)
		}
	})
	t.Run("unknown app", func(t *testing.T) { //nolint:paralleltest // ordered steps share one server
		if status, _, _ := doReq(t, http.MethodGet, app.URL+"/_localslack/apps/A404/delivery-config", "", nil); status != http.StatusNotFound {
			t.Errorf("unknown app status %d", status)
		}
	})
}

// humanAndGeneral returns a non-bot user and #general's id.
func humanAndGeneral(t *testing.T, svc *service.Service) (service.User, string) {
	t.Helper()
	users, err := svc.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	var human service.User
	for _, u := range users {
		if !u.IsBot {
			human = u
			break
		}
	}
	convs, err := svc.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	for _, c := range convs {
		if c.Name == "general" {
			return human, c.ID
		}
	}
	t.Fatal("no #general")
	return human, ""
}
