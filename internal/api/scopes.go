package api

import (
	"net/http"
	"strings"

	"localslack/internal/capabilities"
	"localslack/internal/service"
)

// methodScopes maps method -> accepted scopes from the embedded capabilities manifest.
func methodScopes() map[string][]string {
	m, err := capabilities.Load()
	if err != nil {
		return nil
	}
	out := make(map[string][]string, len(m.Methods))
	for name, meth := range m.Methods {
		out[name] = meth.Scopes
	}
	return out
}

// setScopeHeaders sets Slack's x-oauth-scopes (the caller's grants) and
// x-accepted-oauth-scopes (what the method takes). Headers are omitted when empty.
func (a *API) setScopeHeaders(h http.Header, method string, p service.Principal) {
	if p.TokenKind == service.TokenBot || p.TokenKind == service.TokenUser {
		if s := strings.Join(p.Scopes, ","); s != "" {
			h.Set("X-Oauth-Scopes", s)
		}
	}
	if s := strings.Join(a.scopes[method], ","); s != "" {
		h.Set("X-Accepted-Oauth-Scopes", s)
	}
}
