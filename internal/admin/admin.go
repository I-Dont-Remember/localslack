// Package admin serves the /_localslack control API. One file per feature;
// each exposes mountX(r, d). It must not import api, web or store.
package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
	"localslack/internal/constants"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/faults"
	"localslack/internal/service"
)

// Deps are the admin routes' collaborators. Svc may be nil (health only).
type Deps struct {
	Log *slog.Logger
	Svc *service.Service
	// Optional components; nil when unwired. Streams read the one they own.
	Calls   *calllog.Log
	Faults  *faults.Set
	Events  events.Dispatcher
	Control *control.Control
}

// Mount registers every /_localslack route.
func Mount(r chi.Router, d Deps) {
	mountHealth(r, d)
	mountState(r, d)
	mountApps(r, d)
	mountEvents(r, d)
	mountCalls(r, d)
	mountFaults(r, d)
	mountReset(r, d)
	mountSnapshots(r, d)
	mountSocket(r, d)
}

func writeJSON(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}

func errorBody(code string) map[string]any {
	return map[string]any{"ok": false, "error": code}
}
