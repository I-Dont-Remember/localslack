package socket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"localslack/internal/events"
)

// pingTimeout is how long a ping may wait for its pong.
const pingTimeout = 10 * time.Second

// conn is one app WebSocket.
type conn struct {
	id, appID   string
	connectedAt time.Time
	ws          *websocket.Conn
	done        chan struct{}
	closeOnce   sync.Once

	mu      sync.Mutex
	lastAck *time.Time
	pending map[string]chan struct{}
}

func newConn(appID string, ws *websocket.Conn, now time.Time) *conn {
	return &conn{
		id: newID("SC", 5), appID: appID, connectedAt: now, ws: ws,
		done: make(chan struct{}), pending: map[string]chan struct{}{},
	}
}

type envelope struct {
	EnvelopeID             string          `json:"envelope_id"`
	Payload                json.RawMessage `json:"payload"`
	Type                   string          `json:"type"`
	AcceptsResponsePayload bool            `json:"accepts_response_payload"`
	RetryAttempt           int             `json:"retry_attempt"`
	RetryReason            string          `json:"retry_reason"`
}

func (c *conn) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// deliver sends d as an events_api envelope and waits for its ack.
func (c *conn) deliver(ctx context.Context, d events.Delivery) events.Attempt {
	id := newID("", 8)
	acked := make(chan struct{})
	c.mu.Lock()
	c.pending[id] = acked
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	env := envelope{EnvelopeID: id, Payload: d.Payload, Type: "events_api", RetryAttempt: d.Spec.RetryNum, RetryReason: d.Spec.RetryReason}
	if err := c.send(ctx, env); err != nil {
		if ctx.Err() != nil {
			return timeoutAttempt(ctx)
		}
		c.close()
		return events.Attempt{Error: "socket write failed: " + err.Error(), FailReason: events.ReasonConnection}
	}
	select {
	case <-acked:
		return events.Attempt{Status: http.StatusOK}
	case <-ctx.Done():
		return timeoutAttempt(ctx)
	case <-c.done:
		return events.Attempt{Error: "connection closed before ack", FailReason: events.ReasonConnection}
	}
}

func timeoutAttempt(ctx context.Context) events.Attempt {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return events.Attempt{Error: "timeout", FailReason: events.ReasonTimeout}
	}
	return events.Attempt{Error: "canceled", FailReason: events.ReasonUnknown}
}

// readLoop resolves acks until the socket closes.
func (c *conn) readLoop(ctx context.Context, now func() time.Time) {
	c.ws.SetReadLimit(1 << 20)
	for {
		_, b, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var m struct {
			EnvelopeID string `json:"envelope_id"`
		}
		if json.Unmarshal(b, &m) != nil || m.EnvelopeID == "" {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[m.EnvelopeID]
		if ok {
			delete(c.pending, m.EnvelopeID)
			t := now()
			c.lastAck = &t
		}
		c.mu.Unlock()
		if ok {
			close(ch)
		}
	}
}

// keepAlive pings until the connection ends; a missed pong closes it.
func (c *conn) keepAlive(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil {
				c.close()
				return
			}
		}
	}
}

// close ends the socket without a goodbye; safe to repeat.
func (c *conn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.ws.CloseNow()
	})
}

// goodbye sends a Slack disconnect message (unless reason is empty), then closes politely.
func (c *conn) goodbye(reason string) {
	c.closeOnce.Do(func() {
		close(c.done)
		if reason != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = c.send(ctx, map[string]any{"type": "disconnect", "reason": reason, "debug_info": map[string]any{"host": debugHost}})
		}
		_ = c.ws.Close(websocket.StatusNormalClosure, reason)
	})
}

func (c *conn) info() ConnInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ConnInfo{ID: c.id, AppID: c.appID, ConnectedAt: c.connectedAt, LastAckAt: c.lastAck, Pending: len(c.pending)}
}
