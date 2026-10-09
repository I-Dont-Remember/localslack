package server

import (
	"log/slog"
	"net/http"
	"time"

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
					writeJSON(r.Context(), log, w, http.StatusInternalServerError, errorBody("internal_error"))
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
			log.InfoContext(r.Context(), "request",
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
