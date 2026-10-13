package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"localslack/internal/api"
	"localslack/internal/calllog"
	"localslack/internal/config"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/faults"
	"localslack/internal/service"
	"localslack/internal/socket"
	"localslack/internal/store"
	"localslack/internal/web"
)

// Components are the optional collaborators of the app, wired once in
// newComponents. A new stream adds a field here (and its use in the router /
// admin.Deps) instead of editing shared wiring lines.
type Components struct {
	CallLog *calllog.Log
	Faults  *faults.Set
	Events  events.Dispatcher
	Control *control.Control
	Socket  *socket.Hub

	unsubscribe func()
}

// Option overrides a component before wiring (tests, later streams).
type Option func(*Components)

// WithDispatcher replaces the event dispatcher (default: the real events.Engine).
func WithDispatcher(d events.Dispatcher) Option { return func(c *Components) { c.Events = d } }

// newComponents builds the defaults, applies opts, and subscribes the
// dispatcher to service events.
func newComponents(cfg config.Config, svc *service.Service, log *slog.Logger, opts ...Option) Components {
	c := Components{Faults: faults.NewSet()}
	size := cfg.CallLogSize
	if size <= 0 {
		size = config.DefaultCallLogSize
	}
	c.CallLog = calllog.New(size)
	c.Socket = socket.NewHub(socket.Options{Log: log})
	for _, o := range opts {
		o(&c)
	}
	if c.Events == nil {
		c.Events = newEngine(cfg, svc, log, c.Socket)
	}
	c.Control = control.New(svc, c.Events, control.WithSeed(cfg.Seed), control.WithOnReset(c.resetHooks(svc)...))
	c.unsubscribe = svc.Subscribe(c.Events.Publish)
	return c
}

// resetHooks clear the in-memory state a reset must drop (streams add theirs here).
func (c Components) resetHooks(svc *service.Service) []func() {
	var fns []func()
	if c.Socket != nil { // connections survive a partial reset; apps removed by --all lose theirs
		fns = append(fns, func() { c.Socket.Prune(liveApps(svc)) })
	}
	if c.CallLog != nil {
		fns = append(fns, c.CallLog.Clear)
	}
	if c.Faults != nil {
		fns = append(fns, c.Faults.Clear)
	}
	return fns
}

// apiOptions are the API seams backed by these components.
func (c Components) apiOptions() []api.Option {
	var opts []api.Option
	if c.CallLog != nil {
		opts = append(opts, api.WithRecorder(c.CallLog))
	}
	if c.Faults != nil {
		opts = append(opts, api.WithInterceptor(c.Faults))
	}
	if c.Socket != nil {
		opts = append(opts, api.WithSocket(c.Socket))
	}
	return opts
}

// inspector hands the UI inspector its read/act seams (all non-nil after newComponents).
func (c Components) inspector() web.Inspector {
	return web.Inspector{Calls: c.CallLog, Faults: c.Faults, Events: c.Events, Control: c.Control}
}

// Close drops socket connections and stops event delivery.
func (c Components) Close() error {
	if c.unsubscribe != nil {
		c.unsubscribe()
	}
	if c.Socket != nil {
		c.Socket.Close()
	}
	return c.Events.Close()
}

// App is a fully wired LocalSlack: store, the one service over it, and the router.
type App struct {
	Handler    http.Handler
	Store      *store.Store
	Service    *service.Service
	Components Components
}

// Build is the single place that assembles the app, shared by Run and tests.
// Order matters: the store is migrated and seeded before the service derives
// its ts generator from the stored data. now is injected so tests can pin time.
func Build(ctx context.Context, log *slog.Logger, cfg config.Config, now func() time.Time, opts ...Option) (*App, error) {
	st, err := OpenStore(ctx, log, cfg, now())
	if err != nil {
		return nil, err
	}
	svc, err := service.New(ctx, st, now, "", service.WithStrictTokens(cfg.StrictTokens))
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	comps := newComponents(cfg, svc, log, opts...)
	ui, err := web.New(svc, log, web.WithInspector(comps.inspector()))
	if err != nil {
		_ = errors.Join(comps.Close(), st.Close())
		return nil, err
	}
	if err := registerManifests(ctx, log, cfg, svc, comps.Events); err != nil {
		_ = errors.Join(comps.Close(), st.Close())
		return nil, err
	}
	router := NewRouter(Deps{Log: log, Svc: svc, Web: ui, ListenAddr: cfg.Addr, Components: comps})
	return &App{Handler: router, Store: st, Service: svc, Components: comps}, nil
}

// Close stops components, then releases the store.
func (a *App) Close() error {
	return errors.Join(a.Components.Close(), a.Store.Close())
}

// liveApps reports whether an app id is still registered.
func liveApps(svc *service.Service) func(string) bool {
	live := map[string]bool{}
	for _, a := range svc.Apps() {
		live[a.ID] = true
	}
	return func(id string) bool { return live[id] }
}
