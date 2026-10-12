package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/control"
	"localslack/internal/respond"
	"localslack/internal/scenario"
)

func mountSnapshots(r chi.Router, d Deps) {
	if d.Control == nil {
		return
	}
	r.Post("/_localslack/snapshots", d.createSnapshot)
	r.Get("/_localslack/snapshots", d.listSnapshots)
	r.Get("/_localslack/snapshots/{name}", d.getSnapshot)
	r.Post("/_localslack/snapshots/{name}/restore", d.restoreSnapshot)
	r.Get("/_localslack/diff", d.diff)
}

func snapshotJSON(s control.SnapshotInfo) map[string]any {
	return map[string]any{
		"name": s.Name, "created": s.Created.Format(time.RFC3339),
		"counts": map[string]int{
			"users": s.Counts.Users, "channels": s.Counts.Channels, "dms": s.Counts.DMs,
			"messages": s.Counts.Messages, "reactions": s.Counts.Reactions,
		},
	}
}

func (d Deps) createSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body struct {
		Name string `json:"name"`
	}
	dec := json.NewDecoder(capped(w, r))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		if writeTooLarge(ctx, d.Log, w, err) {
			return
		}
		msg := "snapshot body must be {\"name\": \"...\"}"
		if !errors.Is(err, io.EOF) {
			msg += ": " + err.Error()
		}
		writeError(ctx, d.Log, w, http.StatusBadRequest, "invalid_json", msg)
		return
	}
	info, err := d.Control.CreateSnapshot(ctx, body.Name)
	if err != nil {
		d.snapshotFailed(w, r, err)
		return
	}
	w.Header().Set("Location", "/_localslack/snapshots/"+url.PathEscape(info.Name))
	respond.JSON(ctx, d.Log, w, http.StatusCreated, snapshotJSON(info))
}

func (d Deps) listSnapshots(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	for _, s := range d.Control.Snapshots() {
		items = append(items, snapshotJSON(s))
	}
	respond.JSON(r.Context(), d.Log, w, http.StatusOK, map[string]any{"items": items})
}

func (d Deps) getSnapshot(w http.ResponseWriter, r *http.Request) {
	fx, err := d.Control.SnapshotFixture(chi.URLParam(r, "name"))
	if err != nil {
		d.snapshotFailed(w, r, err)
		return
	}
	respond.JSON(r.Context(), d.Log, w, http.StatusOK, fx)
}

func (d Deps) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	res, err := d.Control.Restore(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		d.snapshotFailed(w, r, err)
		return
	}
	respond.JSON(r.Context(), d.Log, w, http.StatusOK, map[string]any{
		"name": res.Name, "took_ms": res.TookMS,
		"counts": map[string]int{"users": res.Counts.Users, "channels": res.Counts.Channels, "messages": res.Counts.Messages},
	})
}

func (d Deps) diff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		writeError(r.Context(), d.Log, w, http.StatusBadRequest, "invalid_query", "from is required: a snapshot name or \"current\"")
		return
	}
	if to == "" {
		to = control.CurrentRef
	}
	delta, err := d.Control.Diff(r.Context(), from, to)
	if err != nil {
		d.snapshotFailed(w, r, err)
		return
	}
	respond.JSON(r.Context(), d.Log, w, http.StatusOK, delta)
}

// snapshotFailed maps control errors to the documented statuses.
func (d Deps) snapshotFailed(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	var se *scenario.Error
	switch {
	case errors.Is(err, control.ErrInvalidSnapshotName):
		writeError(ctx, d.Log, w, http.StatusUnprocessableEntity, "invalid_snapshot_name",
			"name must be 1-64 letters, digits, '.', '_' or '-', start with a letter or digit, and not be \"current\"")
	case errors.Is(err, control.ErrSnapshotExists):
		writeError(ctx, d.Log, w, http.StatusConflict, "snapshot_exists", "a snapshot with that name exists; pick another name")
	case errors.Is(err, control.ErrSnapshotLimit):
		writeError(ctx, d.Log, w, http.StatusConflict, "snapshot_limit_reached", "too many snapshots; restore or reset to clear them (snapshots are in memory only)")
	case errors.Is(err, control.ErrSnapshotNotFound):
		writeError(ctx, d.Log, w, http.StatusNotFound, "not_found", strings.TrimPrefix(err.Error(), "control: "))
	case errors.Is(err, control.ErrResetInProgress):
		writeError(ctx, d.Log, w, http.StatusConflict, "reset_in_progress", "a reset or restore is running; retry when it finishes")
	case errors.As(err, &se):
		d.invalidScenario(w, r, se, source{code: "invalid_scenario"})
	default:
		d.Log.ErrorContext(ctx, "snapshot", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "internal error; see the server log")
	}
}
