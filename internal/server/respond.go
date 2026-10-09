package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"localslack/internal/constants"
)

// errorBody returns a Slack-shaped error payload.
func errorBody(code string) map[string]any {
	return map[string]any{"ok": false, "error": code}
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}
