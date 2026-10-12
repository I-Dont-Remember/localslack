// Package api implements the Slack Web API surface (/api/{method}) over the service layer.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/constants"
	"localslack/internal/service"
)

// call is one authenticated API invocation.
type call struct {
	p       service.Principal
	params  url.Values
	baseURL string
}

type method func(ctx context.Context, c *call) (map[string]any, error)

// authMode says which credential a method requires.
type authMode int

const (
	authToken authMode = iota // bot/user token (default)
	authNone                  // no credential
	authApp                   // app-level (xapp-) token
)

type spec struct {
	fn   method
	auth authMode
}

// tok wraps a method needing the default token auth.
func tok(fn method) spec { return spec{fn: fn} }

// API serves Slack methods.
type API struct {
	svc      *service.Service
	log      *slog.Logger
	fallback http.HandlerFunc
	methods  map[string]spec
}

// Mount registers /api/{method} (GET and POST). Unknown methods go to fallback.
func Mount(r chi.Router, svc *service.Service, log *slog.Logger, fallback http.HandlerFunc) {
	a := &API{svc: svc, log: log, fallback: fallback}
	a.methods = a.registry()
	r.Get("/api/{method}", a.serve)
	r.Post("/api/{method}", a.serve)
}

// MethodNames lists the Slack methods this server implements (the registry keys),
// sorted. The capabilities manifest is tested against it so the two cannot drift.
func MethodNames() []string {
	names := make([]string, 0)
	for name := range (&API{}).registry() { // method values are bound, never invoked
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// registry merges every file's methods; streams add a funcs-per-file, not lines here.
func (a *API) registry() map[string]spec {
	out := map[string]spec{}
	for _, part := range []map[string]spec{
		a.authMethods(), a.conversationMethods(), a.messageMethods(),
		a.reactionMethods(), a.userMethods(), a.socketMethods(),
	} {
		maps.Copy(out, part)
	}
	return out
}

func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sp, ok := a.methods[chi.URLParam(r, "method")]
	if !ok {
		a.fallback(w, r)
		return
	}
	params, err := parseParams(r)
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	p, err := a.svc.Authenticate(ctx, bearer(r, params))
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	out, err := sp.fn(ctx, &call{p: p, params: params, baseURL: scheme + "://" + r.Host + "/"})
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	out["ok"] = true
	a.write(ctx, w, http.StatusOK, out)
}

func bearer(r *http.Request, params url.Values) string {
	if h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(h)
	}
	return params.Get("token")
}

// fail renders a domain error as Slack does: HTTP 200 with ok:false.
// Unmapped errors are real faults and return 500.
func (a *API) fail(ctx context.Context, w http.ResponseWriter, err error) {
	if code, ok := ErrorCode(err); ok {
		a.write(ctx, w, http.StatusOK, map[string]any{"ok": false, "error": code})
		return
	}
	a.log.ErrorContext(ctx, "api error", slog.Any("error", err))
	a.write(ctx, w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
}

func (a *API) write(ctx context.Context, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}
