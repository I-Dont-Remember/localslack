// Package api implements the Slack Web API surface (/api/{method}) over the service layer.
package api

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
	"localslack/internal/faults"
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
	rec      calllog.Recorder
	icpt     faults.Interceptor
	now      func() time.Time
}

// Option customises Mount.
type Option func(*API)

// WithRecorder sets the call recorder (default: discard).
func WithRecorder(r calllog.Recorder) Option { return func(a *API) { a.rec = r } }

// WithInterceptor sets the fault interceptor (default: never fires).
func WithInterceptor(i faults.Interceptor) Option { return func(a *API) { a.icpt = i } }

// Mount registers /api/{method} (GET and POST). Unknown methods go to fallback.
func Mount(r chi.Router, svc *service.Service, log *slog.Logger, fallback http.HandlerFunc, opts ...Option) {
	a := &API{svc: svc, log: log, fallback: fallback, rec: calllog.Noop{}, icpt: faults.Noop{}, now: time.Now}
	for _, o := range opts {
		o(a)
	}
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
		a.reactionMethods(), a.userMethods(), a.socketMethods(), a.writeMethods(),
	} {
		maps.Copy(out, part)
	}
	return out
}

func bearer(r *http.Request, params url.Values) string {
	if h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(h)
	}
	return params.Get("token")
}
