package api

import (
	"context"
	"errors"
	"net/url"
)

// TicketIssuer mints the one-time ticket in a Socket Mode URL.
type TicketIssuer interface {
	Issue(appID string) string
}

var errSocketUnavailable = errors.New("socket mode is not wired")

// appsConnectionsOpen returns the WebSocket URL for an app-level token's app.
func (a *API) appsConnectionsOpen(_ context.Context, c *call) (map[string]any, error) {
	if a.sock == nil {
		return nil, errSocketUnavailable
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	scheme := "ws"
	if base.Scheme == "https" {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: base.Host, Path: "/_localslack/socket"}
	u.RawQuery = url.Values{"ticket": {a.sock.Issue(c.p.App.ID)}, "app_id": {c.p.App.ID}}.Encode()
	return map[string]any{"url": u.String()}, nil
}

func (a *API) socketMethods() map[string]spec {
	return map[string]spec{"apps.connections.open": {fn: a.appsConnectionsOpen, auth: authApp}}
}
