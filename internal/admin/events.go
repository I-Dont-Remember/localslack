package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"localslack/internal/respond"

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
	r.Post("/_localslack/deliveries/{id}/redeliver", e.redeliverDelivery)
	r.Get("/_localslack/delivery-config", e.getConfig)
	r.Put("/_localslack/delivery-config", e.putConfig)
	r.Patch("/_localslack/delivery-config", e.patchConfig)
	r.Get("/_localslack/apps/{id}/delivery-config", e.getAppConfig)
	r.Put("/_localslack/apps/{id}/delivery-config", e.putAppConfig)
	r.Patch("/_localslack/apps/{id}/delivery-config", e.patchAppConfig)
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
	respond.JSON(r.Context(), e.Log, w, http.StatusOK, map[string]any{"items": items, "next_since": next, "generation": gen})
}

func (e eventsAPI) get(w http.ResponseWriter, r *http.Request) {
	d, ok := e.ctl.Delivery(chi.URLParam(r, "id"))
	if !ok {
		e.problem(w, r, http.StatusNotFound, "delivery_not_found", "no such delivery (it may have aged out of the log)", nil)
		return
	}
	respond.JSON(r.Context(), e.Log, w, http.StatusOK, d)
}

func (e eventsAPI) clear(w http.ResponseWriter, _ *http.Request) {
	e.ctl.ClearDeliveries()
	w.WriteHeader(http.StatusNoContent)
}

func (e eventsAPI) decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(capped(w, r)).Decode(into); err != nil && !errors.Is(err, io.EOF) {
		if writeTooLarge(r.Context(), e.Log, w, err) {
			return false
		}
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
	case errors.Is(err, events.ErrDeliveryNotFound):
		e.problem(w, r, http.StatusNotFound, "delivery_not_found", "no such delivery (it may have aged out of the log)", nil)
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
	respond.JSON(r.Context(), e.Log, w, http.StatusCreated, res)
}

type redeliverBody struct {
	AppID       string `json:"app_id"`
	RetryNum    int    `json:"retry_num"`
	RetryReason string `json:"retry_reason"`
	Signature   string `json:"signature"`
}

func (b redeliverBody) request() events.RedeliverRequest {
	return events.RedeliverRequest{AppID: b.AppID, RetryNum: b.RetryNum, RetryReason: b.RetryReason, Signature: b.Signature}
}

// redeliver resends by event id (unique per app delivery; app_id optional, must match).
func (e eventsAPI) redeliver(w http.ResponseWriter, r *http.Request) {
	var body redeliverBody
	if !e.decode(w, r, &body) {
		return
	}
	res, err := e.ctl.Redeliver(r.Context(), chi.URLParam(r, "event_id"), body.request())
	e.respondRedelivery(w, r, res, err)
}

func (e eventsAPI) redeliverDelivery(w http.ResponseWriter, r *http.Request) {
	var body redeliverBody
	if !e.decode(w, r, &body) {
		return
	}
	res, err := e.ctl.RedeliverDelivery(r.Context(), chi.URLParam(r, "id"), body.request())
	e.respondRedelivery(w, r, res, err)
}

func (e eventsAPI) respondRedelivery(w http.ResponseWriter, r *http.Request, res events.InjectResult, err error) {
	if err != nil {
		e.eventsError(w, r, err)
		return
	}
	respond.JSON(r.Context(), e.Log, w, http.StatusCreated, res)
}

func (e eventsAPI) getConfig(w http.ResponseWriter, r *http.Request) {
	respond.JSON(r.Context(), e.Log, w, http.StatusOK, e.ctl.Config())
}

// putConfig replaces the global config: omitted fields return to their defaults.
func (e eventsAPI) putConfig(w http.ResponseWriter, r *http.Request) {
	e.writeConfig(w, r, e.ctl.ReplaceConfig)
}

// patchConfig merges the given fields into the global config.
func (e eventsAPI) patchConfig(w http.ResponseWriter, r *http.Request) {
	e.writeConfig(w, r, e.ctl.SetConfig)
}

func (e eventsAPI) writeConfig(w http.ResponseWriter, r *http.Request, apply func(events.ConfigPatch) (events.DeliveryConfig, error)) {
	var p events.ConfigPatch
	if !e.decode(w, r, &p) {
		return
	}
	cfg, err := apply(p)
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
	respond.JSON(r.Context(), e.Log, w, http.StatusOK, cfg)
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

// appConfigView is the effective per-app config plus the sparse override that shaped it.
type appConfigView struct {
	events.DeliveryConfig
	Override events.ConfigPatch `json:"override"`
}

func (e eventsAPI) writeAppConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	eff, override := e.ctl.AppConfig(id)
	respond.JSON(r.Context(), e.Log, w, http.StatusOK, appConfigView{DeliveryConfig: eff, Override: override})
}

func (e eventsAPI) getAppConfig(w http.ResponseWriter, r *http.Request) { e.writeAppConfig(w, r) }

// putAppConfig replaces the app's override: fields not given inherit the global config.
func (e eventsAPI) putAppConfig(w http.ResponseWriter, r *http.Request) {
	e.changeAppConfig(w, r, e.ctl.ReplaceAppConfig)
}

// patchAppConfig adds the given fields to the app's override.
func (e eventsAPI) patchAppConfig(w http.ResponseWriter, r *http.Request) {
	e.changeAppConfig(w, r, e.ctl.SetAppConfig)
}

func (e eventsAPI) changeAppConfig(w http.ResponseWriter, r *http.Request, apply func(string, events.ConfigPatch) (events.DeliveryConfig, error)) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	var p events.ConfigPatch
	if !e.decode(w, r, &p) {
		return
	}
	if _, err := apply(id, p); err != nil {
		e.respondConfig(w, r, events.DeliveryConfig{}, err)
		return
	}
	e.writeAppConfig(w, r)
}

func (e eventsAPI) deleteAppConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := e.requireApp(w, r)
	if !ok {
		return
	}
	e.ctl.ClearAppConfig(id)
	w.WriteHeader(http.StatusNoContent)
}
