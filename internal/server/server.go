// Package server implements the HTTP server and route handlers.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"localslack/internal/api"
	"localslack/internal/config"
	"localslack/internal/service"
)

// Deps are the collaborators the router needs. Service-backed routes are
// mounted only when their dependency is set, so health works without any.
type Deps struct {
	Log *slog.Logger
	Svc *service.Service
}

// NewRouter returns the configured HTTP handler for LocalSlack.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, echoRequestID, recoverer(d.Log), requestLogger(d.Log))
	h := handlers{log: d.Log, svc: d.Svc}
	r.Get("/_localslack/health", h.health)
	if d.Svc != nil {
		r.Get("/_localslack/state/conversations", h.stateConversations)
		api.Mount(r, d.Svc, d.Log, h.notImplemented)
	}
	r.NotFound(h.notImplemented)
	r.MethodNotAllowed(h.methodNotAllowed)
	return r
}

// Run starts the HTTP server on addr and blocks until ctx is cancelled or a fatal error occurs.
func Run(ctx context.Context, log *slog.Logger, cfg config.Config) error {
	app, err := Build(ctx, log, cfg, time.Now)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := app.Close(); cerr != nil {
			log.ErrorContext(ctx, "close app", slog.Any("err", cerr))
		}
	}()

	addr := cfg.Addr
	srv := &http.Server{
		Addr:    addr,
		Handler: app.Handler,
	}

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	log.InfoContext(ctx, "server listening", slog.String("addr", addr))

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		log.InfoContext(ctx, "shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}

type handlers struct {
	log *slog.Logger
	svc *service.Service
}

func (h handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusOK, map[string]bool{"ok": true})
}

func (h handlers) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusNotFound, errorBody("not_implemented"))
}

func (h handlers) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusMethodNotAllowed, errorBody("method_not_allowed"))
}
