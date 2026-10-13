package socket_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"localslack/internal/appmanifest"
	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

// e2e drives the real server: registered socket app, real delivery engine.
type e2e struct {
	app *storetest.App
	reg service.App
}

func newE2E(t *testing.T, deliveryConfig string) *e2e {
	t.Helper()
	app := storetest.NewApp(t)
	parsed, err := appmanifest.Parse([]byte(`{"display_information":{"name":"E2E"},"settings":{"socket_mode_enabled":true,"event_subscriptions":{"bot_events":["message.channels"]}}}`))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	reg, err := app.Service.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	e := &e2e{app: app, reg: reg}
	if code, body := e.admin(t, http.MethodPut, "/_localslack/delivery-config", deliveryConfig); code != http.StatusOK {
		t.Fatalf("delivery-config = %d %v", code, body)
	}
	return e
}

func (e *e2e) admin(t *testing.T, verb, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), verb, e.app.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// connect opens a socket through apps.connections.open and swallows the hello.
func (e *e2e) connect(t *testing.T) *websocket.Conn {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.app.URL+"/api/apps.connections.open", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", constants.ContentTypeForm)
	req.Header.Set("Authorization", "Bearer "+e.reg.AppToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct{ URL string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	c, _, err := dialWS(t.Context(), out.URL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if hello := readJSON(t, c); hello["type"] != "hello" {
		t.Fatalf("first message = %v, want hello", hello)
	}
	return c
}

func (e *e2e) inject(t *testing.T, text string) {
	t.Helper()
	body := fmt.Sprintf(`{"app_id":%q,"event":{"type":"message","channel":"C1","text":%q}}`, e.reg.ID, text)
	if code, out := e.admin(t, http.MethodPost, "/_localslack/events", body); code != http.StatusCreated {
		t.Fatalf("inject = %d %v", code, out)
	}
}

// deliveries polls until a delivery in state exists.
func (e *e2e) deliveries(t *testing.T, state string) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		_, body := e.admin(t, http.MethodGet, "/_localslack/deliveries?state="+state, "")
		items, _ := body["items"].([]any)
		if len(items) >= 1 {
			out := make([]map[string]any, len(items))
			for i, it := range items {
				out[i], _ = it.(map[string]any)
			}
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("want a delivery in state %q, have %v", state, items)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func eventText(env map[string]any) string {
	p, _ := env["payload"].(map[string]any)
	ev, _ := p["event"].(map[string]any)
	s, _ := ev["text"].(string)
	return s
}

func TestEngineDeliversOverSocket(t *testing.T) {
	t.Parallel()
	e := newE2E(t, `{"timeout_ms":2000,"retry_delays_ms":[]}`)
	c := e.connect(t)
	e.inject(t, "hi")

	env := readJSON(t, c)
	if env["type"] != "events_api" || eventText(env) != "hi" || env["retry_attempt"] != float64(0) {
		t.Fatalf("envelope = %v, want events_api for 'hi', retry_attempt 0", env)
	}
	ack(t, c, env["envelope_id"].(string))

	d := e.deliveries(t, "delivered")[0]
	attempts, _ := d["attempts"].([]any)
	first, _ := attempts[0].(map[string]any)
	if d["transport"] != "socket" || len(attempts) != 1 || first["status"] != float64(200) || first["error"] != "" {
		t.Errorf("delivery = %v, want one acked socket attempt", d)
	}
}

func TestEngineRetriesWithoutAck(t *testing.T) {
	t.Parallel()
	e := newE2E(t, `{"timeout_ms":150,"retry_delays_ms":[0]}`)
	c := e.connect(t)
	e.inject(t, "again")

	first := readJSON(t, c) // never acked
	second := readJSON(t, c)
	if second["retry_attempt"] != float64(1) || second["retry_reason"] != "http_timeout" || eventText(second) != "again" {
		t.Fatalf("retry envelope = %v, want retry_attempt 1 / http_timeout for the same event", second)
	}
	if first["envelope_id"] == second["envelope_id"] {
		t.Error("retry reused the envelope_id, want a new one per attempt")
	}
	ack(t, c, second["envelope_id"].(string))

	d := e.deliveries(t, "delivered")[0]
	attempts, _ := d["attempts"].([]any)
	a0, _ := attempts[0].(map[string]any)
	if len(attempts) != 2 || a0["error"] != "timeout" {
		t.Errorf("attempts = %v, want a timed-out attempt then an acked one", attempts)
	}
}

func TestEngineNoConnectionRetriesThenDelivers(t *testing.T) {
	t.Parallel()
	e := newE2E(t, `{"timeout_ms":1000,"retry_delays_ms":[0,200,200,200,200,200,200]}`)
	e.inject(t, "late")

	d := e.deliveries(t, "retrying")[0]
	attempts, _ := d["attempts"].([]any)
	a0, _ := attempts[0].(map[string]any)
	if a0["error"] != "no_socket_connection" {
		t.Fatalf("first attempt = %v, want error no_socket_connection", a0)
	}

	c := e.connect(t)
	env := readJSON(t, c)
	if eventText(env) != "late" || env["retry_attempt"].(float64) < 1 {
		t.Fatalf("envelope = %v, want the retried 'late' event", env)
	}
	ack(t, c, env["envelope_id"].(string))
	e.deliveries(t, "delivered")
}

func TestEnginePreservesOrder(t *testing.T) {
	t.Parallel()
	e := newE2E(t, `{"timeout_ms":2000,"retry_delays_ms":[]}`)
	c := e.connect(t)
	const n = 6
	for i := range n {
		e.inject(t, fmt.Sprintf("m%d", i))
	}
	var got []string
	for range n {
		env := readJSON(t, c)
		got = append(got, eventText(env))
		ack(t, c, env["envelope_id"].(string))
	}
	for i, text := range got {
		if text != fmt.Sprintf("m%d", i) {
			t.Fatalf("order = %v, want m0..m%d in publish order", got, n-1)
		}
	}
}

func TestEngineRoundRobinsConnections(t *testing.T) {
	t.Parallel()
	e := newE2E(t, `{"timeout_ms":2000,"retry_delays_ms":[]}`)
	a, b := e.connect(t), e.connect(t)
	inbox := make(chan inboxMsg, 8)
	forward(t, "a", a, inbox)
	forward(t, "b", b, inbox)
	var who []string
	for i := range 4 {
		e.inject(t, fmt.Sprintf("r%d", i))
		select {
		case m := <-inbox:
			who = append(who, m.who)
			ack(t, m.c, m.env["envelope_id"].(string))
		case <-time.After(wait):
			t.Fatal("no envelope")
		}
	}
	if who[0] == who[1] || who[0] != who[2] || who[1] != who[3] {
		t.Errorf("recipients = %v, want alternation", who)
	}
}
