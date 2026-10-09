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
)

// NewRouter returns the configured HTTP handler for LocalSlack.
func NewRouter(log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, echoRequestID, recoverer(log), requestLogger(log))
	h := handlers{log: log}
	r.Get("/_localslack/health", h.health)
	r.NotFound(h.notImplemented)
	r.MethodNotAllowed(h.methodNotAllowed)
	return r
}

// Run starts the HTTP server on addr and blocks until ctx is cancelled or a fatal error occurs.
func Run(ctx context.Context, log *slog.Logger, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: NewRouter(log),
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

type handlers struct{ log *slog.Logger }

func (h handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusOK, map[string]bool{"ok": true})
}

func (h handlers) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusNotFound, errorBody("not_implemented"))
}

func (h handlers) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), h.log, w, http.StatusMethodNotAllowed, errorBody("method_not_allowed"))
}
