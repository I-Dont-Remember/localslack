// Package api implements the Slack Web API surface (/api/{method}) over the service layer.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/constants"
	"localslack/internal/service"
)

// call is one authenticated API invocation.
type call struct {
	user    service.User
	params  url.Values
	baseURL string
}

type method func(ctx context.Context, c *call) (map[string]any, error)

// API serves Slack methods.
type API struct {
	svc      *service.Service
	log      *slog.Logger
	fallback http.HandlerFunc
	methods  map[string]method
}

// Mount registers /api/{method} (GET and POST). Unknown methods go to fallback.
func Mount(r chi.Router, svc *service.Service, log *slog.Logger, fallback http.HandlerFunc) {
	a := &API{svc: svc, log: log, fallback: fallback}
	a.methods = a.registry()
	r.Get("/api/{method}", a.serve)
	r.Post("/api/{method}", a.serve)
}

func (a *API) registry() map[string]method {
	return map[string]method{
		"auth.test":  a.authTest,
		"users.list": a.usersList,
	}
}

func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, ok := a.methods[chi.URLParam(r, "method")]
	if !ok {
		a.fallback(w, r)
		return
	}
	params, err := parseParams(r)
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	user, err := a.svc.Authenticate(ctx, bearer(r, params))
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	out, err := m(ctx, &call{user: user, params: params, baseURL: scheme + "://" + r.Host + "/"})
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
