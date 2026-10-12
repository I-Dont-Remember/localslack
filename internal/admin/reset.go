package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/control"
	"localslack/internal/service"
)

const maxResetBody = 1 << 20

func mountReset(r chi.Router, d Deps) {
	if d.Control == nil {
		return
	}
	r.Post("/_localslack/reset", d.reset)
}

// resetBody is the optional POST body. Scenario and Generate belong to the
// scenario stream; until it lands they are recognised and refused.
type resetBody struct {
	Seed     string          `json:"seed"`
	Scenario json.RawMessage `json:"scenario"`
	Generate json.RawMessage `json:"generate"`
}

func (d Deps) reset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := parseAll(r)
	if err != nil {
		writeError(ctx, d.Log, w, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	var body resetBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxResetBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		code := "invalid_json"
		if isUnknownField(err) {
			code = "invalid_reset"
		}
		writeError(ctx, d.Log, w, http.StatusBadRequest, code, "reset body: "+err.Error())
		return
	}
	switch {
	case body.Scenario != nil && body.Generate != nil:
		writeError(ctx, d.Log, w, http.StatusBadRequest, "invalid_reset", "give scenario or generate, not both")
		return
	case body.Scenario != nil || body.Generate != nil:
		writeError(ctx, d.Log, w, http.StatusUnprocessableEntity, "not_supported_yet", "scenario and generate resets are not available yet; reset with an optional seed")
		return
	}
	res, err := d.Control.Reset(ctx, control.ResetOpts{All: all, Seed: body.Seed})
	if err != nil {
		d.resetFailed(w, r, err)
		return
	}
	writeJSON(ctx, d.Log, w, http.StatusOK, map[string]any{
		"mode": res.Mode, "generation": res.Generation, "seed": res.Seed, "took_ms": res.TookMS,
		"counts": map[string]int{"users": res.Counts.Users, "channels": res.Counts.Channels, "messages": res.Counts.Messages},
	})
}

func (d Deps) resetFailed(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	switch {
	case errors.Is(err, control.ErrResetInProgress):
		writeError(ctx, d.Log, w, http.StatusConflict, "reset_in_progress", "another reset is running; retry when it finishes")
	case errors.Is(err, service.ErrResetIDCollision):
		writeError(ctx, d.Log, w, http.StatusUnprocessableEntity, "invalid_reset", err.Error())
	default:
		d.Log.ErrorContext(ctx, "reset", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "reset failed; see the server log")
	}
}

func parseAll(r *http.Request) (bool, error) {
	s := r.URL.Query().Get("all")
	if s == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, errors.New("all must be true or false")
	}
	return b, nil
}

// isUnknownField reports the decoder's DisallowUnknownFields error, which has no type.
func isUnknownField(err error) bool { return strings.HasPrefix(err.Error(), "json: unknown field") }
