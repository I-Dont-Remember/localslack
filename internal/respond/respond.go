// Package respond writes JSON responses and errors. It is a leaf shared by
// server and admin so both use one writer and one /_localslack error shape.
package respond

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"localslack/internal/constants"
)

// AdminPrefix is the path prefix of the testing/control API.
const AdminPrefix = "/_localslack/"

// IsAdminPath reports whether p belongs to the /_localslack control API, whose
// errors are {"error":{"code","message"}} with real HTTP statuses. Everything
// else (/api/*, the UI) keeps Slack's {"ok":false,"error":code}.
func IsAdminPath(p string) bool { return strings.HasPrefix(p, AdminPrefix) || p+"/" == AdminPrefix }

// JSON writes v as a JSON response with the given status.
func JSON(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}

// Error writes the /_localslack error shape; details may be nil.
func Error(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, code, message string, details []map[string]string) {
	e := map[string]any{"code": code, "message": message}
	if len(details) > 0 {
		e["details"] = details
	}
	JSON(ctx, log, w, status, map[string]any{"error": e})
}

// SlackError writes Slack's error shape: {"ok":false,"error":code}.
func SlackError(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, code string) {
	JSON(ctx, log, w, status, map[string]any{"ok": false, "error": code})
}

// ForRequest writes an error in the shape the request's path expects.
func ForRequest(ctx context.Context, log *slog.Logger, w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if IsAdminPath(r.URL.Path) {
		Error(ctx, log, w, status, code, message, nil)
		return
	}
	SlackError(ctx, log, w, status, code)
}
