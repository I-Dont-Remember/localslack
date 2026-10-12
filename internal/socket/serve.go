package socket

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/coder/websocket"

	"localslack/internal/respond"
)

const debugHost = "localslack"

// ServeWS upgrades a Socket Mode connection: the Origin must not be cross-site
// and the ticket must be unused, unexpired and issued for ?app_id.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if crossSite(r) {
		respond.Error(ctx, h.log, w, http.StatusForbidden, "forbidden_origin", "cross-origin WebSocket refused", nil)
		return
	}
	q := r.URL.Query()
	appID := q.Get("app_id")
	if appID == "" || !h.redeem(q.Get("ticket"), appID) {
		respond.Error(ctx, h.log, w, http.StatusUnauthorized, "invalid_ticket", "ticket is missing, expired, already used, or not for this app", nil)
		return
	}
	// Origin was checked above, so the library's own same-host check is skipped.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		h.log.WarnContext(ctx, "socket upgrade failed", slog.Any("error", err))
		return
	}
	c := newConn(appID, ws, h.now())
	n, ok := h.add(c)
	if !ok {
		_ = ws.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	defer h.remove(c)
	defer c.close()
	hello := map[string]any{
		"type": "hello", "num_connections": n,
		"debug_info":      map[string]any{"host": debugHost, "build_number": 0, "approximate_connection_time": 18060},
		"connection_info": map[string]any{"app_id": appID},
	}
	if err := c.send(ctx, hello); err != nil {
		return
	}
	// The hijacked request context ends with the handler; the loop outlives it only via ctx.
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	go c.keepAlive(rctx, h.pingInterval)
	c.readLoop(rctx, h.now)
}

// crossSite reports a browser request from another origin. Non-browser clients
// (Bolt, curl) send neither header.
func crossSite(r *http.Request) bool {
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
