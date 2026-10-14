package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/respond"
	"localslack/internal/socket"
)

const socketPath = "/_localslack/socket"

// mountSocket serves the Socket Mode WebSocket and its controls, see docs/TESTING_API.md.
func mountSocket(r chi.Router, d Deps) {
	if d.Socket == nil {
		return
	}
	h := socketHandlers{Deps: d}
	r.Get(socketPath, d.Socket.ServeWS)
	r.Get(socketPath+"/connections", h.list)
	r.Post(socketPath+"/disconnect", h.disconnect)
}

type socketHandlers struct{ Deps }

func (h socketHandlers) list(w http.ResponseWriter, r *http.Request) {
	respond.JSON(r.Context(), h.Log, w, http.StatusOK, map[string]any{"items": h.Socket.Connections()})
}

type disconnectBody struct {
	ConnectionID string `json:"connection_id"`
	Reason       string `json:"reason"`
	Drop         bool   `json:"drop"`
}

func (h socketHandlers) disconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw, err := io.ReadAll(capped(w, r))
	if err != nil && writeTooLarge(ctx, h.Log, w, err) {
		return
	}
	var body disconnectBody
	if err != nil || (len(raw) > 0 && json.Unmarshal(raw, &body) != nil) {
		writeError(ctx, h.Log, w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return
	}
	if body.Reason == "" {
		body.Reason = socket.ReasonRefreshRequested
	}
	switch _, err := h.Socket.Disconnect(body.ConnectionID, body.Reason, body.Drop); {
	case errors.Is(err, socket.ErrNoSuchConnection):
		writeError(ctx, h.Log, w, http.StatusNotFound, "connection_not_found", "no such socket connection")
	case errors.Is(err, socket.ErrInvalidReason):
		writeError(ctx, h.Log, w, http.StatusUnprocessableEntity, "invalid_reason", "reason must be refresh_requested, warning or link_disabled")
	default:
		writeNoContent(w)
	}
}
