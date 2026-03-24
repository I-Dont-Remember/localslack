// Package server implements the HTTP server and route handlers.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Run starts the HTTP server on addr and blocks until ctx is cancelled or a fatal error occurs.
func Run(ctx context.Context, addr string) error {
	r := chi.NewRouter()
	r.Get("/_localslack/health", healthHandler)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	slog.Info("server listening", "addr", addr)

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		slog.ErrorContext(r.Context(), "encode health response", "error", err)
	}
}
