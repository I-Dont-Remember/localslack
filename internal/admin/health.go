package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func mountHealth(r chi.Router, d Deps) {
	r.Get("/_localslack/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(r.Context(), d.Log, w, http.StatusOK, map[string]bool{"ok": true})
	})
}
