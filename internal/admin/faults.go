package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"

	"localslack/internal/faults"
)

const faultsPath = "/_localslack/faults"

// Known JSON keys per object, to report unknown ones with a pointer.
//
//nolint:gochecknoglobals // immutable lookup tables
var faultKeys = map[string][]string{
	"":       {"method", "match", "nth", "every", "times", "once", "action"},
	"match":  {"channel", "app_id", "user_id", "token_kind"},
	"action": {"status", "error", "retry_after", "delay_ms", "body"},
}

// mountFaults serves the fault-rule CRUD, see docs/TESTING_API.md.
func mountFaults(r chi.Router, d Deps) {
	if d.Faults == nil {
		return
	}
	h := faultHandlers{Deps: d}
	r.Post(faultsPath, h.create)
	r.Get(faultsPath, h.list)
	r.Delete(faultsPath, h.clear)
	r.Delete(faultsPath+"/{id}", h.remove)
}

type faultHandlers struct{ Deps }

func (h faultHandlers) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(capped(w, r))
	if err != nil && writeTooLarge(ctx, h.Log, w, err) {
		return
	}
	if err != nil || !json.Valid(body) {
		writeError(ctx, h.Log, w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return
	}
	spec, details := decodeSpec(body)
	if len(details) > 0 {
		writeInvalid(ctx, h, w, details)
		return
	}
	rule, err := h.Faults.Add(spec)
	var ve *faults.ValidationError
	if errors.As(err, &ve) {
		writeInvalid(ctx, h, w, ve.Details)
		return
	}
	w.Header().Set("Location", faultsPath+"/"+rule.ID)
	respond.JSON(ctx, h.Log, w, http.StatusCreated, rule)
}

func (h faultHandlers) list(w http.ResponseWriter, r *http.Request) {
	respond.JSON(r.Context(), h.Log, w, http.StatusOK, map[string]any{"items": h.Faults.List()})
}

func (h faultHandlers) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.Faults.Delete(id) {
		writeError(r.Context(), h.Log, w, http.StatusNotFound, "not_found", "no fault "+id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h faultHandlers) clear(w http.ResponseWriter, _ *http.Request) {
	h.Faults.Clear()
	w.WriteHeader(http.StatusNoContent)
}

func writeInvalid(ctx context.Context, h faultHandlers, w http.ResponseWriter, details []faults.Detail) {
	respond.JSON(ctx, h.Log, w, http.StatusUnprocessableEntity, map[string]any{
		"error": map[string]any{"code": "invalid_fault", "message": "invalid fault rule", "details": details},
	})
}

// decodeSpec reports unknown keys and type errors as pointers; the Spec is
// only meaningful when no details come back.
func decodeSpec(body []byte) (faults.Spec, []faults.Detail) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return faults.Spec{}, []faults.Detail{{Pointer: "", Message: "body must be a JSON object"}}
	}
	details := unknownKeys(raw, "")
	for _, sub := range []string{"match", "action"} {
		if obj, ok := raw[sub].(map[string]any); ok {
			details = append(details, unknownKeys(obj, sub)...)
		}
	}
	var spec faults.Spec
	var typeErr *json.UnmarshalTypeError
	if err := json.Unmarshal(body, &spec); errors.As(err, &typeErr) {
		details = append(details, faults.Detail{
			Pointer: "/" + strings.ReplaceAll(typeErr.Field, ".", "/"),
			Message: "expected " + typeErr.Type.String(),
		})
	}
	return spec, details
}

func unknownKeys(obj map[string]any, scope string) []faults.Detail {
	prefix := "/"
	if scope != "" {
		prefix = "/" + scope + "/"
	}
	var out []faults.Detail
	for k := range obj {
		if !slices.Contains(faultKeys[scope], k) {
			out = append(out, faults.Detail{Pointer: prefix + k, Message: "unknown field"})
		}
	}
	slices.SortFunc(out, func(a, b faults.Detail) int { return strings.Compare(a.Pointer, b.Pointer) })
	return out
}
