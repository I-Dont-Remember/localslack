// Package socket is the Socket Mode server: tickets, WebSocket connections,
// and the events.Transport that delivers envelopes over them. It must not
// import api, web, admin or store.
package socket

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"localslack/internal/events"
)

// Defaults for Options.
const (
	DefaultTicketTTL    = 30 * time.Second
	DefaultPingInterval = 30 * time.Second
)

// Domain errors of the control methods.
var (
	ErrNoSuchConnection = errors.New("no such socket connection")
	ErrInvalidReason    = errors.New("invalid disconnect reason")
)

// Options configure NewHub; zero values get defaults.
type Options struct {
	// Now is the ticket clock (default time.Now).
	Now          func() time.Time
	TicketTTL    time.Duration
	PingInterval time.Duration
	Log          *slog.Logger
}

type ticket struct {
	appID   string
	expires time.Time
}

// Hub owns every Socket Mode connection and the one-time tickets that open them.
type Hub struct {
	now          func() time.Time
	ttl          time.Duration
	pingInterval time.Duration
	log          *slog.Logger

	mu      sync.Mutex
	tickets map[string]ticket
	conns   []*conn // oldest first
	rr      map[string]int
	closed  bool
}

// NewHub returns an empty Hub. Close it to drop every connection.
func NewHub(o Options) *Hub {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.TicketTTL <= 0 {
		o.TicketTTL = DefaultTicketTTL
	}
	if o.PingInterval <= 0 {
		o.PingInterval = DefaultPingInterval
	}
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Hub{now: o.Now, ttl: o.TicketTTL, pingInterval: o.PingInterval, log: o.Log, tickets: map[string]ticket{}, rr: map[string]int{}}
}

// Issue mints a one-time ticket for appID, valid for the ticket TTL.
func (h *Hub) Issue(appID string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	t := base64.RawURLEncoding.EncodeToString(b)
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.tickets {
		if now.After(v.expires) {
			delete(h.tickets, k)
		}
	}
	h.tickets[t] = ticket{appID: appID, expires: now.Add(h.ttl)}
	return t
}

// redeem consumes ticket t; it is valid once, for appID, before it expires.
func (h *Hub) redeem(t, appID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	tk, ok := h.tickets[t]
	delete(h.tickets, t)
	return ok && tk.appID == appID && !h.now().After(tk.expires)
}

func (h *Hub) add(c *conn) (n int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0, false
	}
	h.conns = append(h.conns, c)
	return h.countLocked(c.appID), true
}

func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns = slices.DeleteFunc(h.conns, func(x *conn) bool { return x == c })
}

func (h *Hub) countLocked(appID string) int {
	n := 0
	for _, c := range h.conns {
		if c.appID == appID {
			n++
		}
	}
	return n
}

// pick chooses appID's next connection, round-robin.
func (h *Hub) pick(appID string) *conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	var mine []*conn
	for _, c := range h.conns {
		if c.appID == appID {
			mine = append(mine, c)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	i := h.rr[appID] % len(mine)
	h.rr[appID] = i + 1
	return mine[i]
}

// Deliver implements events.Transport: one envelope to one of the app's
// connections, then wait for its ack (or the ctx deadline).
func (h *Hub) Deliver(ctx context.Context, d events.Delivery) events.Attempt {
	c := h.pick(d.AppID)
	if c == nil {
		return events.Attempt{Error: "no_socket_connection", FailReason: events.ReasonConnection, RetryReason: d.Spec.RetryReason}
	}
	start := time.Now()
	a := c.deliver(ctx, d)
	a.LatencyMS = time.Since(start).Milliseconds()
	a.RetryReason = d.Spec.RetryReason
	return a
}

// Close drops every connection and refuses new ones.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	conns := slices.Clone(h.conns)
	h.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
}

func newID(prefix string, n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // see Issue
	return prefix + hex.EncodeToString(b)
}
