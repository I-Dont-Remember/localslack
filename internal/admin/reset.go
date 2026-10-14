package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"

	"localslack/internal/control"
	"localslack/internal/scenario"
	"localslack/internal/service"
)

func mountReset(r chi.Router, d Deps) {
	if d.Control == nil {
		return
	}
	r.Post("/_localslack/reset", d.reset)
}

// resetBody is the optional POST body. Scenario is a fixture object or the path
// of a .json/.toml/.yaml/.yml file on the server's machine; Generate is a recipe.
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
	dec := json.NewDecoder(capped(w, r))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		if writeTooLarge(ctx, d.Log, w, err) {
			return
		}
		code := "invalid_json"
		if isUnknownField(err) {
			code = "invalid_reset"
		}
		writeError(ctx, d.Log, w, http.StatusBadRequest, code, "reset body: "+err.Error())
		return
	}
	sc, src, ok := d.resetSource(w, r, body)
	if !ok {
		return
	}
	res, err := d.Control.Reset(ctx, control.ResetOpts{All: all, Seed: body.Seed, Scenario: sc})
	if err != nil {
		d.resetFailed(w, r, err, src)
		return
	}
	respond.JSON(ctx, d.Log, w, http.StatusOK, map[string]any{
		"mode": res.Mode, "generation": res.Generation, "seed": res.Seed, "took_ms": res.TookMS,
		"counts": map[string]int{"users": res.Counts.Users, "channels": res.Counts.Channels, "messages": res.Counts.Messages},
	})
}

func (d Deps) resetFailed(w http.ResponseWriter, r *http.Request, err error, src source) {
	ctx := r.Context()
	var se *scenario.Error
	switch {
	case errors.As(err, &se):
		d.invalidScenario(w, r, se, src)
	case errors.Is(err, control.ErrResetInProgress):
		writeError(ctx, d.Log, w, http.StatusConflict, "reset_in_progress", "another reset is running; retry when it finishes")
	case errors.Is(err, service.ErrResetIDCollision):
		writeError(ctx, d.Log, w, http.StatusUnprocessableEntity, "invalid_reset", err.Error())
	default:
		d.Log.ErrorContext(ctx, "reset", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "reset failed; see the server log")
	}
}

// source says where a reset's data came from, to word errors and prefix pointers.
type source struct{ code, prefix string }

func srcScenario() source { return source{"invalid_scenario", "/scenario"} }
func srcGenerate() source { return source{"invalid_generate", ""} }

// resetSource decodes the body's scenario or generate (at most one) into a
// fixture; nil means the default seed. It answers the request itself on failure.
func (d Deps) resetSource(w http.ResponseWriter, r *http.Request, body resetBody) (*scenario.Fixture, source, bool) {
	sc, gen := bytes.TrimSpace(body.Scenario), bytes.TrimSpace(body.Generate)
	hasSc, hasGen := len(sc) > 0 && string(sc) != "null", len(gen) > 0 && string(gen) != "null"
	switch {
	case hasSc && hasGen:
		writeError(r.Context(), d.Log, w, http.StatusBadRequest, "invalid_reset", "give scenario or generate, not both")
		return nil, source{}, false
	case hasSc:
		fx, ok := d.decodeScenario(w, r, sc)
		return fx, srcScenario(), ok
	case hasGen:
		fx, ok := d.decodeGenerate(w, r, gen)
		return fx, srcGenerate(), ok
	}
	return nil, source{}, true
}

func (d Deps) decodeScenario(w http.ResponseWriter, r *http.Request, raw []byte) (*scenario.Fixture, bool) {
	var fx scenario.Fixture
	var err error
	switch raw[0] {
	case '"':
		var path string
		if err = json.Unmarshal(raw, &path); err == nil {
			fx, err = scenario.LoadFile(path)
		}
	case '{':
		fx, err = scenario.Parse(raw, scenario.JSON)
	default:
		writeError(r.Context(), d.Log, w, http.StatusBadRequest, "invalid_reset", "scenario must be a fixture object or the path of a scenario file")
		return nil, false
	}
	var se *scenario.Error
	switch {
	case err == nil:
		return &fx, true
	case errors.As(err, &se):
		d.invalidScenario(w, r, se, srcScenario())
	case errors.Is(err, scenario.ErrSyntax):
		writeError(r.Context(), d.Log, w, http.StatusUnprocessableEntity, srcScenario().code, err.Error())
	default:
		writeError(r.Context(), d.Log, w, http.StatusBadRequest, "invalid_reset", "scenario: "+err.Error())
	}
	return nil, false
}

func (d Deps) decodeGenerate(w http.ResponseWriter, r *http.Request, raw []byte) (*scenario.Fixture, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var g scenario.Generate
	if err := dec.Decode(&g); err != nil {
		writeError(r.Context(), d.Log, w, http.StatusBadRequest, "invalid_reset", "generate: "+err.Error())
		return nil, false
	}
	return &scenario.Fixture{Version: scenario.Version, Generate: &g}, true
}

// invalidScenario answers 422 with every problem as {pointer,message} details.
func (d Deps) invalidScenario(w http.ResponseWriter, r *http.Request, se *scenario.Error, src source) {
	details := make([]map[string]string, 0, len(se.Issues))
	for _, i := range se.Issues {
		details = append(details, map[string]string{"pointer": src.prefix + i.Pointer, "message": i.Message})
	}
	msg := "the fixture has problems; nothing was changed"
	if len(details) > 0 {
		msg += ": " + details[0]["pointer"] + " " + details[0]["message"]
	}
	respond.Error(r.Context(), d.Log, w, http.StatusUnprocessableEntity, src.code, msg, details)
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
