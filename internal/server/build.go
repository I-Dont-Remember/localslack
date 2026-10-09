package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"localslack/internal/config"
	"localslack/internal/service"
	"localslack/internal/store"
	"localslack/internal/web"
)

// App is a fully wired LocalSlack: store, the one service over it, and the router.
type App struct {
	Handler http.Handler
	Store   *store.Store
	Service *service.Service
}

// Build is the single place that assembles the app, shared by Run and tests.
// Order matters: the store is migrated and seeded before the service derives
// its ts generator from the stored data. now is injected so tests can pin time.
func Build(ctx context.Context, log *slog.Logger, cfg config.Config, now func() time.Time) (*App, error) {
	st, err := OpenStore(ctx, log, cfg, now())
	if err != nil {
		return nil, err
	}
	svc, err := service.New(ctx, st, now, "")
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	ui, err := web.New(svc, log)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	return &App{Handler: NewRouter(Deps{Log: log, Svc: svc, Web: ui, ListenAddr: cfg.Addr}), Store: st, Service: svc}, nil
}

// Close releases the store.
func (a *App) Close() error { return a.Store.Close() }
