package socket

import (
	"slices"
	"time"
)

// Disconnect reasons Slack sends (https://api.slack.com/apis/socket-mode).
const (
	ReasonRefreshRequested = "refresh_requested"
	ReasonWarning          = "warning"
	ReasonLinkDisabled     = "link_disabled"
)

// ConnInfo describes one connection (GET /_localslack/socket/connections).
type ConnInfo struct {
	ID          string     `json:"id"`
	AppID       string     `json:"app_id"`
	ConnectedAt time.Time  `json:"connected_at"`
	LastAckAt   *time.Time `json:"last_ack_at"`
	Pending     int        `json:"pending"`
}

// ValidReason reports whether r is a disconnect reason Slack uses.
func ValidReason(r string) bool {
	return r == ReasonRefreshRequested || r == ReasonWarning || r == ReasonLinkDisabled
}

// Connections lists live connections, oldest first.
func (h *Hub) Connections() []ConnInfo {
	h.mu.Lock()
	conns := slices.Clone(h.conns)
	h.mu.Unlock()
	out := make([]ConnInfo, 0, len(conns))
	for _, c := range conns {
		out = append(out, c.info())
	}
	return out
}

// Disconnect closes connection id (every connection when id is empty) and
// returns how many it closed. Unless drop is set it first sends Slack's
// disconnect message with reason; drop closes silently, like a lost network.
func (h *Hub) Disconnect(id, reason string, drop bool) (int, error) {
	if !drop && !ValidReason(reason) {
		return 0, ErrInvalidReason
	}
	h.mu.Lock()
	var targets []*conn
	for _, c := range h.conns {
		if id == "" || c.id == id {
			targets = append(targets, c)
		}
	}
	if id != "" && len(targets) == 0 {
		h.mu.Unlock()
		return 0, ErrNoSuchConnection
	}
	h.conns = slices.DeleteFunc(h.conns, func(c *conn) bool { return slices.Contains(targets, c) })
	h.mu.Unlock()
	for _, c := range targets {
		if drop {
			go c.goodbye("")
			continue
		}
		go c.goodbye(reason)
	}
	return len(targets), nil
}

// Prune closes every connection whose app keep rejects (the app was removed),
// and forgets its tickets.
func (h *Hub) Prune(keep func(appID string) bool) {
	h.mu.Lock()
	var gone []*conn
	for _, c := range h.conns {
		if !keep(c.appID) {
			gone = append(gone, c)
		}
	}
	h.conns = slices.DeleteFunc(h.conns, func(c *conn) bool { return slices.Contains(gone, c) })
	for t, tk := range h.tickets {
		if !keep(tk.appID) {
			delete(h.tickets, t)
		}
	}
	h.mu.Unlock()
	for _, c := range gone {
		c.close()
	}
}
