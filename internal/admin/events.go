package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/events"
)

const maxPollTimeout = 30 * time.Second

func mountEvents(r chi.Router, d Deps) {
	if d.Svc == nil {
		return
	}
	ctl, ok := d.controller()
	if !ok {
		return
	}
	e := eventsAPI{Deps: d, ctl: ctl}
	r.Get("/_localslack/deliveries", e.list)
	r.Get("/_localslack/deliveries/{id}", e.get)
	r.Delete("/_localslack/deliveries", e.clear)
	r.Post("/_localslack/events", e.inject)
	r.Post("/_localslack/events/{event_id}/redeliver", e.redeliver)
	r.Get("/_localslack/delivery-config", e.getConfig)
	r.Put("/_localslack/delivery-config", e.putConfig)
	r.Get("/_localslack/apps/{id}/delivery-config", e.getAppConfig)
	r.Put("/_localslack/apps/{id}/delivery-config", e.putAppConfig)
	r.Delete("/_localslack/apps/{id}/delivery-config", e.deleteAppConfig)
}

type eventsAPI struct {
	Deps
	ctl events.Controller
}

func (e eventsAPI) list(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	since, err1 := intParam(v, "since")
	limit, err2 := intParam(v, "limit")
	minN, err3 := intParam(v, "min")
	timeoutMS, err4 := intParam(v, "timeout_ms")
	if err := errors.Join(err1, err2, err3, err4); err != nil {
		e.problem(w, r, http.StatusBadRequest, "invalid_query", err.Error(), nil)
		return
	}
	f := events.Filter{
		AppID: v.Get("app_id"), EventType: v.Get("event_type"), State: v.Get("state"),
		Since: int64(since), Limit: limit,
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if minN > 0 && v.Get("timeout_ms") == "" {
		timeout = defaultPollTimeout
	}
	timeout = min(timeout, maxPollTimeout)
	items, next, gen := e.ctl.WaitDeliveries(r.Context(), f, minN, timeout)
	writeJSON(r.Context(), e.Log, w, http.StatusOK, map[string]any{"items": items, "next_since": next, "generation": gen})
}

func (e eventsAPI) get(w http.ResponseWriter, r *http.Request) {
	d, ok := e.ctl.Delivery(chi.URLParam(r, "id"))
	if !ok {
		e.problem(w, r, http.StatusNotFound, "delivery_not_found", "no such delivery (it may have aged out of the log)", nil)
		return
	}
	writeJSON(r.Context(), e.Log, w, http.StatusOK, d)
}

func (e eventsAPI) clear(w http.ResponseWriter, _ *http.Request) {
	e.ctl.ClearDeliveries()
	w.WriteHeader(http.StatusNoContent)
}

func (e eventsAPI) decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxManifestBytes)).Decode(into); err != nil && !errors.Is(err, io.EOF) {
		e.problem(w, r, http.StatusBadRequest, "malformed_request", "invalid request body: "+err.Error(), nil)
		return false
	}
	return true
}

// eventsError maps the dispatcher's errors to responses.
func (e eventsAPI) eventsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, events.ErrAppNotFound):
		e.notFound(w, r)
	case errors.Is(err, events.ErrEventNotFound):
		e.problem(w, r, http.StatusNotFound, "event_not_found", "no such event (it may have aged out of the log)", nil)
	case errors.Is(err, events.ErrInvalidEvent):
		e.problem(w, r, http.StatusUnprocessableEntity, "invalid_event",
			`event.type is required and signature must be one of valid, invalid, stale, missing`, nil)
	default:
		e.internal(w, r, "events control", err)
	}
}

func (e eventsAPI) inject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Event     map[string]any `json:"event"`
		AppID     string         `json:"app_id"`
		Signature string         `json:"signature"`
		Envelope  map[string]any `json:"envelope"`
	}
	if !e.decode(w, r, &body) {
		return
	}
	res, err := e.ctl.Inject(r.Context(), events.InjectRequest{
		Event: body.Event, AppID: body.AppID, Signature: body.Signature, Envelope: body.Envelope,
	})
	if err != nil {
		e.eventsError(w, r, err)
		return
	}
	writeJSON(r.Context(), e.Log, w, http.StatusCreated, res)
}

func (e eventsAPI) redeliver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppID       string `json:"app_id"`
		RetryNum    int    `json:"retry_num"`
		RetryReason string `json:"retry_reason"`
		Signature   string `json:"signature"`
	}
	if !e.decode(w, r, &body) {
		return
	}
	res, err := e.ctl.Redeliver(r.Context(), chi.URLParam(r, "event_id"), events.RedeliverRequest{
		AppID: body.AppID, RetryNum: body.RetryNum, RetryReason: body.RetryReason, Signature: body.Signature,
	})
	if err != nil {
		e.eventsError(w, r, err)
		return
	}
	writeJSON(r.Context(), e.Log, w, http.StatusCreated, res)
}

func (e eventsAPI) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(r.Context(), e.Log, w, http.StatusOK, e.ctl.Config())
}

func (e eventsAPI) putConfig(w http.ResponseWriter, r *http.Request) {
	var p events.ConfigPatch
	if !e.decode(w, r, &p) {
		return
	}
	cfg, err := e.ctl.SetConfig(p)
	e.respondConfig(w, r, cfg, err)
}

func (e eventsAPI) respondConfig(w http.ResponseWriter, r *http.Request, cfg events.DeliveryConfig, err error) {
	var ce *events.ConfigError
	if errors.As(err, &ce) {
		e.problem(w, r, http.StatusUnprocessableEntity, "invalid_delivery_config", ce.Error(),
			[]map[string]string{{"message": ce.Message, "pointer": "/" + ce.Field}})
		return
	}
	if err != nil {
		e.internal(w, r, "delivery config", err)
		return
	}
	writeJSON(r.Context(), e.Log, w, http.StatusOK, cfg)
}

func (e eventsAPI) requireApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if _, err := e.Svc.App(r.Context(), id); err != nil {
		e.appError(w, r, "load app", err)
		return "", false
	}
	return id, true
}

func (e eventsAPI) appError(w http.ResponseWriter, r *http.Request, what string, err error) {
	appsAPI{Deps: e.Deps}.appError(w, r, what, err)
}

func (e eventsAPI) getAppConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	cfg, _ := e.ctl.AppConfig(id)
	writeJSON(r.Context(), e.Log, w, http.StatusOK, cfg)
}

func (e eventsAPI) putAppConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	var p events.ConfigPatch
	if !e.decode(w, r, &p) {
		return
	}
	cfg, err := e.ctl.SetAppConfig(id, p)
	e.respondConfig(w, r, cfg, err)
}

func (e eventsAPI) deleteAppConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	e.ctl.ClearAppConfig(id)
	w.WriteHeader(http.StatusNoContent)
}
