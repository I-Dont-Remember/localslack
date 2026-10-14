package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"localslack/internal/appmanifest"
	"localslack/internal/calllog"
	"localslack/internal/events"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

const manifestFmt = `{"display_information":{"name":"NAME"},"features":{"bot_user":{"display_name":"NAME"}},
"settings":{"event_subscriptions":{"request_url":"URL","bot_events":["message.channels"]}}}`

func registerApp(t *testing.T, app *storetest.App, name, requestURL string) service.App {
	t.Helper()
	doc := strings.NewReplacer("NAME", name, "URL", requestURL).Replace(manifestFmt)
	parsed, err := appmanifest.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a, err := app.Service.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	return a
}

func TestInspectorPageAndNav(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)

	status, page, _ := b.get("/ui/inspector")
	if status != http.StatusOK {
		t.Fatalf("inspector status = %d", status)
	}
	contains(t, page,
		`/ui/static/htmx.min.js`,
		`data-testid="inspector-calls"`, `hx-get="/ui/inspector/calls"`,
		`data-testid="inspector-deliveries"`, `data-testid="inspector-apps"`,
		`data-testid="inspector-faults"`, `data-testid="inspector-controls"`,
		`data-testid="inspector-sockets"`, `data-testid="inspector-snapshots"`,
		`hx-get="/ui/inspector/sockets"`, `hx-get="/ui/inspector/snapshots"`,
	)
	_, chat, _ := b.get("/ui/c/" + w.general)
	contains(t, chat, `data-testid="nav-inspector"`, `href="/ui/inspector"`)
}

func TestInspectorCalls(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)
	rec := app.Components.CallLog
	rec.Record(calllog.CallRecord{Time: time.Unix(1, 0), Method: "chat.postMessage", Status: 200, OK: true, Params: map[string]any{"channel": "C1", "text": "hello-params"}, Response: map[string]any{"ts": "1.1"}})
	rec.Record(calllog.CallRecord{Method: "chat.update", Status: 200, Error: "message_not_found"})
	rec.Record(calllog.CallRecord{Method: "chat.postMessage", Status: 429, Error: "ratelimited", FaultID: "f_9"})
	rec.Record(calllog.CallRecord{Method: "nope.nada", Status: 200, Error: "unknown_method", NotImplemented: true})

	tests := []struct {
		name    string
		query   string
		want    []string
		notWant []string
	}{
		{"all", "", []string{"chat.postMessage", "chat.update", "nope.nada", "hello-params", "message_not_found", "ratelimited"}, nil},
		{"badges", "", []string{`data-testid="badge-fault"`, "f_9", `data-testid="badge-not-implemented"`}, nil},
		{"method filter", "?method=chat.update", []string{"chat.update"}, []string{"nope.nada", "hello-params"}},
		{"ok filter", "?ok=true", []string{"hello-params"}, []string{"chat.update", "nope.nada"}},
		{"not ok filter", "?ok=false", []string{"chat.update", "nope.nada"}, []string{"hello-params"}},
		{"channel filter", "?channel=C1", []string{"hello-params"}, []string{"chat.update"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, body, _ := b.get("/ui/inspector/calls" + tt.query)
			if status != http.StatusOK {
				t.Fatalf("status = %d", status)
			}
			contains(t, body, tt.want...)
			for _, nw := range tt.notWant {
				if strings.Contains(body, nw) {
					t.Errorf("body has %q, want absent", nw)
				}
			}
		})
	}

	t.Run("newest first", func(t *testing.T) {
		t.Parallel()
		_, body, _ := b.get("/ui/inspector/calls")
		if strings.Index(body, "nope.nada") > strings.Index(body, "chat.update") {
			t.Error("newest call should be listed first")
		}
	})
}

func TestInspectorCallsCapsAtNewest100(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	for range 130 {
		app.Components.CallLog.Record(calllog.CallRecord{Method: "m.old", OK: true})
	}
	app.Components.CallLog.Record(calllog.CallRecord{Method: "m.newest", OK: true})
	_, body, _ := newBrowser(t, app).get("/ui/inspector/calls")
	if got := strings.Count(body, `data-testid="call-row"`); got != 100 {
		t.Errorf("rows = %d, want 100", got)
	}
	contains(t, body, "m.newest")
}

func TestInspectorFaults(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)

	status, body, _ := b.post("/ui/inspector/faults", url.Values{
		"method": {"chat.postMessage"}, "times": {"2"}, "status": {"429"}, "error": {"ratelimited"}, "retry_after": {"3"},
	})
	if status != http.StatusOK {
		t.Fatalf("add status = %d: %s", status, body)
	}
	contains(t, body, "f_1", "chat.postMessage", "429", "ratelimited", "retry_after=3", "times=2", `data-testid="fault-row"`)
	if rules := app.Components.Faults.List(); len(rules) != 1 || rules[0].Times != 2 {
		t.Fatalf("rules = %+v, want one with times=2", rules)
	}

	_, body, _ = b.post("/ui/inspector/faults", url.Values{"method": {""}, "status": {"500"}})
	contains(t, body, `data-testid="fault-error"`)
	if len(app.Components.Faults.List()) != 1 {
		t.Error("invalid rule must not be stored")
	}

	_, body, _ = b.post("/ui/inspector/faults/f_1/delete", nil)
	if strings.Contains(body, `data-testid="fault-row"`) || len(app.Components.Faults.List()) != 0 {
		t.Errorf("fault not deleted: %s", body)
	}
}

func TestInspectorApps(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)
	blocked := registerApp(t, app, "Remote Bot", "https://example.com/slack/events")
	ready := registerApp(t, app, "Local Bot", "http://localhost:9/slack/events")

	_, body, _ := b.get("/ui/inspector/apps")
	contains(t, body, "Remote Bot", "Local Bot", blocked.BotUserID, blocked.BotToken, ready.SigningSecret,
		`data-state="blocked"`, `data-state="ready"`, "not loopback/private", "http://localhost:9/slack/events", "message.channels")

	status, body, _ := b.post("/ui/inspector/apps/"+blocked.ID+"/deliver_to", url.Values{"deliver_to": {"http://localhost:3000"}})
	if status != http.StatusOK {
		t.Fatalf("deliver_to status = %d", status)
	}
	contains(t, body, "http://localhost:3000/slack/events")
	if strings.Count(body, `data-state="blocked"`) != 0 {
		t.Errorf("app should be ready after deliver_to:\n%s", body)
	}

	_, body, _ = b.post("/ui/inspector/apps/"+blocked.ID+"/deliver_to", url.Values{"deliver_to": {"not a url"}})
	contains(t, body, `data-testid="apps-error"`)

	_, body, _ = b.post("/ui/inspector/apps/"+blocked.ID+"/deliver_to", url.Values{"deliver_to": {""}})
	contains(t, body, `data-state="blocked"`)
}

func TestInspectorDeliveries(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)
	a := registerApp(t, app, "Hook Bot", "https://example.com/e")
	ctl := app.Components.Events.(events.Controller)
	res, err := ctl.Inject(t.Context(), events.InjectRequest{AppID: a.ID, Event: map[string]any{"type": "message", "text": "payload-text"}})
	if err != nil || len(res.Deliveries) != 1 {
		t.Fatalf("Inject = %+v, %v", res, err)
	}
	ref := res.Deliveries[0]

	_, body, _ := b.get("/ui/inspector/deliveries")
	contains(t, body, `data-testid="delivery-row"`, ref.EventID, a.ID, "message", "http", "payload-text", `data-testid="redeliver"`)

	_, body, _ = b.get("/ui/inspector/deliveries?event_id=" + ref.EventID)
	contains(t, body, ref.EventID)
	_, body, _ = b.get("/ui/inspector/deliveries?event_id=nope")
	if strings.Contains(body, `data-testid="delivery-row"`) {
		t.Error("event_id filter should hide non-matching deliveries")
	}

	status, _, _ := b.post("/ui/inspector/deliveries/"+ref.DeliveryID+"/redeliver", url.Values{"signature": {"invalid"}})
	if status != http.StatusOK {
		t.Fatalf("redeliver status = %d", status)
	}
	items, _, _ := ctl.Deliveries(events.Filter{})
	if len(items) != 2 {
		t.Errorf("deliveries = %d, want 2 after redeliver", len(items))
	}
}

func TestInspectorControls(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)
	ctl := app.Components.Events.(events.Controller)
	registerApp(t, app, "Keeper", "http://localhost:9/e")

	tests := []struct {
		name string
		form url.Values
		want func(events.DeliveryConfig) bool
	}{
		{"signature stale", url.Values{"signature": {"stale"}}, func(c events.DeliveryConfig) bool { return c.Signature == "stale" }},
		{"pause", url.Values{"paused": {"true"}}, func(c events.DeliveryConfig) bool { return c.Paused }},
		{"resume", url.Values{"paused": {"false"}}, func(c events.DeliveryConfig) bool { return !c.Paused }},
		{"timeout", url.Values{"timeout_ms": {"1234"}}, func(c events.DeliveryConfig) bool { return c.TimeoutMS == 1234 }},
	}
	for _, tt := range tests {
		status, body, _ := b.post("/ui/inspector/config", tt.form)
		if status != http.StatusOK || !tt.want(ctl.Config()) {
			t.Errorf("%s: status %d, config %+v", tt.name, status, ctl.Config())
		}
		contains(t, body, `data-testid="events-dropped"`)
	}
	_, body, _ := b.post("/ui/inspector/config", url.Values{"signature": {"bogus"}})
	contains(t, body, `data-testid="controls-error"`)

	_, body, _ = b.get("/ui/inspector/controls")
	contains(t, body, "1234", `data-testid="events-dropped"`)

	_, body, _ = b.post("/ui/inspector/reset", url.Values{"seed": {"abc"}})
	contains(t, body, `data-testid="reset-result"`, "partial", "abc")
	if len(app.Service.Apps()) != 1 {
		t.Error("partial reset must keep apps")
	}
	if ctl.Config().Signature != "valid" {
		t.Error("reset must restore delivery-config defaults")
	}

	_, body, _ = b.post("/ui/inspector/reset", url.Values{"all": {"true"}})
	contains(t, body, "all")
	if len(app.Service.Apps()) != 0 {
		t.Error("reset all must remove apps")
	}
}
