package admin

import (
	"net/http"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"
)

func mountHealth(r chi.Router, d Deps) {
	r.Get("/_localslack/health", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(r.Context(), d.Log, w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
