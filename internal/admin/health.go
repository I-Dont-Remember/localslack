package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/respond"
)

// dropCounter is implemented by dispatchers that count events lost to a full queue.
type dropCounter interface{ Dropped() int64 }

func mountHealth(r chi.Router, d Deps) {
	r.Get("/_localslack/health", func(w http.ResponseWriter, r *http.Request) {
		var dropped int64
		if c, ok := d.Events.(dropCounter); ok {
			dropped = c.Dropped()
		}
		respond.JSON(r.Context(), d.Log, w, http.StatusOK, map[string]any{"status": healthyStatus, "events_dropped": dropped})
	})
}
