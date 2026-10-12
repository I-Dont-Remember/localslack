package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
	evID := res["event_id"].(string)
	if ds := res["deliveries"].([]any); len(ds) != 1 || ds[0].(map[string]any)["app_id"] != id {
		t.Fatalf("deliveries = %v", res["deliveries"])
	}
	doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=1&timeout_ms=5000", "", nil)

	status, _, again := doReq(t, http.MethodPost, app.URL+"/_localslack/events/"+evID+"/redeliver", constants.ContentTypeJSON,
		map[string]any{"retry_num": 2, "retry_reason": "http_timeout"})
	if status != http.StatusCreated || again["event_id"] != evID {
		t.Fatalf("redeliver status %d %v", status, again)
	}
	_, _, list := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=2&timeout_ms=5000", "", nil)
	if len(list["items"].([]any)) != 2 {
		t.Fatalf("deliveries = %v", list["items"])
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.hdrs) != 2 || fake.hdrs[1].Get("X-Slack-Retry-Num") != "2" {
		t.Errorf("headers = %v", fake.hdrs)
	}

	for name, tc := range map[string]struct {
		path   string
		body   map[string]any
		status int
		code   string
	}{
		"inject no type":     {"/_localslack/events", map[string]any{"event": map[string]any{}}, 422, "invalid_event"},
		"inject unknown app": {"/_localslack/events", map[string]any{"event": map[string]any{"type": "x"}, "app_id": "A404"}, 404, "app_not_found"},
		"redeliver unknown":  {"/_localslack/events/Ev404/redeliver", map[string]any{}, 404, "event_not_found"},
	} {
		status, _, body := doReq(t, http.MethodPost, app.URL+tc.path, constants.ContentTypeJSON, tc.body)
		if status != tc.status || sub(body, "error")["code"] != tc.code {
			t.Errorf("%s: status %d body %v, want %d %s", name, status, body, tc.status, tc.code)
		}
	}
}

func TestDeliveryConfigGlobalAndPerApp(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newRecApp(t)
	id := register(t, app.URL, "Cfg", fake.URL, "message.channels")
	base := app.URL + "/_localslack/delivery-config"

	status, _, def := doReq(t, http.MethodGet, base, "", nil)
	if status != http.StatusOK || def["signature"] != "valid" || def["paused"] != false || def["timeout_ms"] == nil {
		t.Fatalf("default config %d %v", status, def)
	}
	status, _, put := doReq(t, http.MethodPut, base, constants.ContentTypeJSON, map[string]any{"signature": "stale", "paused": true})
	if status != http.StatusOK || put["signature"] != "stale" || put["paused"] != true || put["timeout_ms"] != def["timeout_ms"] {
		t.Errorf("put config %d %v", status, put)
	}
	if status, _, bad := doReq(t, http.MethodPut, base, constants.ContentTypeJSON, map[string]any{"signature": "bogus"}); status != 422 || sub(bad, "error")["code"] != "invalid_delivery_config" {
		t.Errorf("bad config %d %v", status, bad)
	}
	doReq(t, http.MethodPut, base, constants.ContentTypeJSON, map[string]any{"signature": "valid", "paused": false})

	appBase := app.URL + "/_localslack/apps/" + id + "/delivery-config"
	_, _, put = doReq(t, http.MethodPut, appBase, constants.ContentTypeJSON, map[string]any{"signature": "missing"})
	_, _, got := doReq(t, http.MethodGet, appBase, "", nil)
	if put["signature"] != "missing" || got["signature"] != "missing" {
		t.Errorf("per-app config %v / %v", put, got)
	}
	if _, _, g := doReq(t, http.MethodGet, base, "", nil); g["signature"] != "valid" {
		t.Errorf("per-app override leaked to global: %v", g)
	}
	if status, _, _ := doReq(t, http.MethodDelete, appBase, "", nil); status != http.StatusNoContent {
		t.Errorf("delete override status %d", status)
	}
	if _, _, g := doReq(t, http.MethodGet, appBase, "", nil); g["signature"] != "valid" {
		t.Errorf("override survived delete: %v", g)
	}
	if status, _, _ := doReq(t, http.MethodGet, app.URL+"/_localslack/apps/A404/delivery-config", "", nil); status != http.StatusNotFound {
		t.Errorf("unknown app status %d", status)
	}
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
