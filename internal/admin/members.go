package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/service"
)

func mountMembers(r chi.Router, d Deps) {
	if d.Svc == nil {
		return
	}
	r.Put("/_localslack/channels/{id}/members/{user_id}", d.setMember(true))
	r.Delete("/_localslack/channels/{id}/members/{user_id}", d.setMember(false))
}

// setMember adds or removes a member in any conversation (test setup; no events, idempotent).
func (d Deps) setMember(member bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		err := d.Svc.SetMembership(ctx, chi.URLParam(r, "id"), chi.URLParam(r, "user_id"), member)
		switch {
		case errors.Is(err, service.ErrChannelNotFound):
			writeError(ctx, d.Log, w, http.StatusNotFound, "not_found", "no such channel")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(ctx, d.Log, w, http.StatusNotFound, "not_found", "no such user")
		case err != nil:
			d.Log.ErrorContext(ctx, "set member", slog.Any("error", err))
			writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "internal error; see the server log")
		default:
			writeNoContent(w)
		}
	}
}
