package socket_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
	"localslack/internal/socket"
)

const wait = 5 * time.Second

type fixture struct {
	hub *socket.Hub
	srv *httptest.Server
	now atomic.Int64 // unix seconds
}

func newFixture(t *testing.T, mutate ...func(*socket.Options)) *fixture {
	t.Helper()
	f := &fixture{}
	f.now.Store(1_700_000_000)
	o := socket.Options{Now: func() time.Time { return time.Unix(f.now.Load(), 0) }}
	for _, m := range mutate {
		m(&o)
	}
	f.hub = socket.NewHub(o)
	f.srv = httptest.NewServer(http.HandlerFunc(f.hub.ServeWS))
	t.Cleanup(func() { f.srv.Close(); f.hub.Close() })
	return f
}

func (f *fixture) url(ticket, appID string) string {
	return "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/socket?ticket=" + ticket + "&app_id=" + appID
}

// dial connects with a fresh ticket and returns the client plus its hello.
func (f *fixture) dial(t *testing.T, appID string) (*websocket.Conn, map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	c, _, err := dialWS(ctx, f.url(f.hub.Issue(appID), appID), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c, readJSON(t, c)
}

// dialWS dials with extra headers and returns the handshake status (0 if none).
func dialWS(ctx context.Context, url string, header http.Header) (*websocket.Conn, int, error) {
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	return c, status, err
}

func readJSON(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return m
}

func ack(t *testing.T, c *websocket.Conn, envelopeID string) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"envelope_id": envelopeID})
	if err := c.Write(t.Context(), websocket.MessageText, b); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

func delivery(app string) events.Delivery {
	return events.Delivery{AppID: app, Payload: []byte(`{"type":"event_callback","event_id":"Ev1"}`)}
}

// deliverAsync runs Deliver in the background and returns its result channel.
func (f *fixture) deliverAsync(ctx context.Context, d events.Delivery) <-chan events.Attempt {
	out := make(chan events.Attempt, 1)
	go func() { out <- f.hub.Deliver(ctx, d) }()
	return out
}

func result(t *testing.T, ch <-chan events.Attempt) events.Attempt {
	t.Helper()
	select {
	case a := <-ch:
		return a
	case <-time.After(wait):
		t.Fatal("timed out waiting for Deliver")
		return events.Attempt{}
	}
}

func waitCount(t *testing.T, h *socket.Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for len(h.Connections()) != want {
		if time.Now().After(deadline) {
			t.Fatalf("connections = %d, want %d", len(h.Connections()), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHello(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, got := f.dial(t, "A1")
	if got["type"] != "hello" || got["num_connections"] != float64(1) {
		t.Errorf("hello = %v, want type hello and num_connections 1", got)
	}
	want := map[string]any{"app_id": "A1"}
	if d := cmp.Diff(want, got["connection_info"]); d != "" {
		t.Errorf("connection_info mismatch (-want +got):\n%s", d)
	}
	if _, ok := got["debug_info"].(map[string]any); !ok {
		t.Errorf("hello has no debug_info: %v", got)
	}
	_, second := f.dial(t, "A1")
	if second["num_connections"] != float64(2) {
		t.Errorf("second hello num_connections = %v, want 2", second["num_connections"])
	}
}

func TestEnvelopeAndAck(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c, _ := f.dial(t, "A1")
	d := delivery("A1")
	d.Spec.RetryNum, d.Spec.RetryReason = 2, "timeout"

	res := f.deliverAsync(t.Context(), d)
	env := readJSON(t, c)
	id, _ := env["envelope_id"].(string)
	if id == "" {
		t.Fatalf("envelope has no envelope_id: %v", env)
	}
	want := map[string]any{
		"envelope_id": id, "type": "events_api", "accepts_response_payload": false,
		"retry_attempt": float64(2), "retry_reason": "timeout",
		"payload": map[string]any{"type": "event_callback", "event_id": "Ev1"},
	}
	if diff := cmp.Diff(want, env); diff != "" {
		t.Errorf("envelope mismatch (-want +got):\n%s", diff)
	}
	ack(t, c, id)
	got := result(t, res)
	if !got.OK() || got.Status != 200 {
		t.Errorf("attempt = %+v, want acknowledged (status 200, no error)", got)
	}
}

func TestDeliverFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		connect    bool
		ctxTimeout time.Duration
		after      func(*testing.T, *websocket.Conn)
		wantErr    string
		wantReason string
	}{
		{name: "no connection", wantErr: "no_socket_connection", wantReason: events.ReasonConnection},
		{name: "no ack", connect: true, ctxTimeout: 100 * time.Millisecond, wantErr: "timeout", wantReason: events.ReasonTimeout},
		{
			name: "closed before ack", connect: true, ctxTimeout: wait,
			after:   func(t *testing.T, c *websocket.Conn) { t.Helper(); readJSON(t, c); _ = c.CloseNow() },
			wantErr: "connection closed before ack", wantReason: events.ReasonConnection,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			var c *websocket.Conn
			if tt.connect {
				c, _ = f.dial(t, "A1")
			}
			ctx := t.Context()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			res := f.deliverAsync(ctx, delivery("A1"))
			if tt.after != nil {
				tt.after(t, c)
			}
			got := result(t, res)
			if got.OK() || got.Error != tt.wantErr || got.FailReason != tt.wantReason {
				t.Errorf("attempt = {err %q reason %q ok %v}, want err %q reason %q", got.Error, got.FailReason, got.OK(), tt.wantErr, tt.wantReason)
			}
		})
	}
}

func TestUpgradeRejections(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	used := f.hub.Issue("A1")
	c, _, err := dialWS(t.Context(), f.url(used, "A1"), nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	_ = c.CloseNow()
	expired := f.hub.Issue("A1")
	f.now.Add(31)

	tests := []struct {
		name   string
		url    string
		header http.Header
		want   int
	}{
		{"ticket reuse", f.url(used, "A1"), nil, http.StatusUnauthorized},
		{"expired ticket", f.url(expired, "A1"), nil, http.StatusUnauthorized},
		{"unknown ticket", f.url("nope", "A1"), nil, http.StatusUnauthorized},
		{"missing ticket", f.url("", "A1"), nil, http.StatusUnauthorized},
		{"app mismatch", f.url(f.hub.Issue("A1"), "A2"), nil, http.StatusUnauthorized},
		{"cross-site origin", f.url(f.hub.Issue("A1"), "A1"), http.Header{"Origin": {"https://evil.example"}}, http.StatusForbidden},
		{"fetch-site cross-site", f.url(f.hub.Issue("A1"), "A1"), http.Header{"Sec-Fetch-Site": {"cross-site"}}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, status, err := dialWS(t.Context(), tt.url, tt.header)
			if err == nil {
				t.Fatalf("dial succeeded, want HTTP %d", tt.want)
			}
			if status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

func TestSameOriginBrowserAllowed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	host := strings.TrimPrefix(f.srv.URL, "http://")
	c, _, err := dialWS(t.Context(), f.url(f.hub.Issue("A1"), "A1"), http.Header{"Origin": {"http://" + host}})
	if err != nil {
		t.Fatalf("same-origin dial: %v", err)
	}
	_ = c.CloseNow()
}

type inboxMsg struct {
	who string
	c   *websocket.Conn
	env map[string]any
}

// forward pumps c's messages into inbox until it closes.
func forward(t *testing.T, who string, c *websocket.Conn, inbox chan<- inboxMsg) {
	t.Helper()
	go func() {
		for {
			_, raw, err := c.Read(context.Background())
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				inbox <- inboxMsg{who: who, c: c, env: m}
			}
		}
	}()
}

func TestRoundRobin(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	inbox := make(chan inboxMsg, 8)
	a, _ := f.dial(t, "A1")
	b, _ := f.dial(t, "A1")
	forward(t, "a", a, inbox)
	forward(t, "b", b, inbox)

	var got []string
	for range 4 {
		res := f.deliverAsync(t.Context(), delivery("A1"))
		select {
		case m := <-inbox:
			got = append(got, m.who)
			ack(t, m.c, m.env["envelope_id"].(string))
		case <-time.After(wait):
			t.Fatal("no envelope received")
		}
		if r := result(t, res); !r.OK() {
			t.Fatalf("attempt failed: %+v", r)
		}
	}
	if got[0] == got[1] || got[0] != got[2] || got[1] != got[3] {
		t.Errorf("recipients = %v, want strict alternation between the two connections", got)
	}
}

func TestOnlyOwnAppReceives(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dial(t, "A1")
	got := f.hub.Deliver(t.Context(), delivery("A2"))
	if got.Error != "no_socket_connection" {
		t.Errorf("error = %q, want no_socket_connection (connection belongs to another app)", got.Error)
	}
}

func TestDisconnect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reason  string
		drop    bool
		wantMsg map[string]any
	}{
		{name: "refresh", reason: "refresh_requested", wantMsg: map[string]any{"type": "disconnect", "reason": "refresh_requested"}},
		{name: "warning", reason: "warning", wantMsg: map[string]any{"type": "disconnect", "reason": "warning"}},
		{name: "link disabled", reason: "link_disabled", wantMsg: map[string]any{"type": "disconnect", "reason": "link_disabled"}},
		{name: "drop", reason: "refresh_requested", drop: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			c, _ := f.dial(t, "A1")
			if _, err := f.hub.Disconnect("", tt.reason, tt.drop); err != nil {
				t.Fatalf("Disconnect: %v", err)
			}
			if tt.wantMsg != nil {
				got := readJSON(t, c)
				delete(got, "debug_info")
				if diff := cmp.Diff(tt.wantMsg, got); diff != "" {
					t.Errorf("disconnect message mismatch (-want +got):\n%s", diff)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), wait)
			defer cancel()
			if _, raw, err := c.Read(ctx); err == nil {
				t.Errorf("connection still open after disconnect, got %s", raw)
			}
			waitCount(t, f.hub, 0)
		})
	}
}

func TestDisconnectErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dial(t, "A1")
	if _, err := f.hub.Disconnect("nope", "warning", false); !errors.Is(err, socket.ErrNoSuchConnection) {
		t.Errorf("unknown id: err = %v, want ErrNoSuchConnection", err)
	}
	if _, err := f.hub.Disconnect("", "bogus", false); !errors.Is(err, socket.ErrInvalidReason) {
		t.Errorf("bad reason: err = %v, want ErrInvalidReason", err)
	}
	if n := len(f.hub.Connections()); n != 1 {
		t.Errorf("connections = %d, want 1 (failed disconnects must not close anything)", n)
	}
}

func TestDisconnectOneOfMany(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.dial(t, "A1")
	f.dial(t, "A1")
	conns := f.hub.Connections()
	if len(conns) != 2 {
		t.Fatalf("connections = %d, want 2", len(conns))
	}
	target := conns[0].ID
	if n, err := f.hub.Disconnect(target, "warning", true); err != nil || n != 1 {
		t.Fatalf("Disconnect = %d, %v, want 1, nil", n, err)
	}
	waitCount(t, f.hub, 1)
	if left := f.hub.Connections()[0].ID; left == target {
		t.Errorf("remaining connection is the disconnected one (%s)", left)
	}
}

func TestConnectionsList(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c, _ := f.dial(t, "A1")
	got := f.hub.Connections()
	if len(got) != 1 || got[0].AppID != "A1" || got[0].ID == "" || got[0].LastAckAt != nil || got[0].Pending != 0 {
		t.Fatalf("connections = %+v, want one fresh A1 connection", got)
	}
	res := f.deliverAsync(t.Context(), delivery("A1"))
	env := readJSON(t, c)
	if p := f.hub.Connections()[0].Pending; p != 1 {
		t.Errorf("pending while unacked = %d, want 1", p)
	}
	ack(t, c, env["envelope_id"].(string))
	result(t, res)
	after := f.hub.Connections()[0]
	if after.Pending != 0 || after.LastAckAt == nil {
		t.Errorf("after ack = %+v, want pending 0 and last_ack_at set", after)
	}
	_ = c.Close(websocket.StatusNormalClosure, "bye")
	waitCount(t, f.hub, 0)
}

func TestPruneClosesRemovedApps(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	gone, _ := f.dial(t, "A1")
	f.dial(t, "A2")
	f.hub.Prune(func(appID string) bool { return appID == "A2" })
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	if _, _, err := gone.Read(ctx); err == nil {
		t.Error("A1 connection still open after Prune")
	}
	waitCount(t, f.hub, 1)
}

func TestPingsKeepConnectionAlive(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(o *socket.Options) { o.PingInterval = 20 * time.Millisecond })
	c, _ := f.dial(t, "A1")
	// A client that keeps reading answers pings automatically; the connection must survive several.
	forward(t, "a", c, make(chan inboxMsg, 1))
	time.Sleep(200 * time.Millisecond)
	if n := len(f.hub.Connections()); n != 1 {
		t.Errorf("connections = %d after pings, want 1", n)
	}
}
