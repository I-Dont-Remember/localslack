package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5/middleware"
)

// recoverer converts handler panics into a logged JSON 500.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(r.Context(), "handler panic",
						slog.Any("panic", rec),
						slog.String("request_id", middleware.GetReqID(r.Context())))
					respond.ForRequest(r.Context(), log, w, r, http.StatusInternalServerError, "internal_error", "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// requestLogger logs one structured line per request.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodGet {
				level = slog.LevelDebug // UI polls every 2s per open tab
			}
			log.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())))
		})
	}
}

// echoRequestID copies the request ID (set by chi's RequestID) to the response.
func echoRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}
