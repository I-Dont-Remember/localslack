package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/faults"
	"localslack/internal/service"
)

// inspectorLimit is how many newest rows each inspector list shows.
const inspectorLimit = 100

// CallSource is the call log the inspector reads (*calllog.Log).
type CallSource interface {
	Query(ctx context.Context, q calllog.Query) calllog.Result
}

// FaultStore is the fault rule set the inspector edits (*faults.Set).
type FaultStore interface {
	List() []faults.Rule
	Add(spec faults.Spec) (faults.Rule, error)
	Delete(id string) bool
}

// Resetter wipes workspace state (*control.Control).
type Resetter interface {
	Reset(ctx context.Context, o control.ResetOpts) (control.ResetResult, error)
}

// Inspector are the optional components behind /ui/inspector. Any field may be
// nil (the matching panel then says so). Events is type-asserted to
// events.Controller, so a noop dispatcher simply has no controls.
type Inspector struct {
	Calls   CallSource
	Faults  FaultStore
	Events  events.Dispatcher
	Control Resetter
	// Snapshots drives the snapshot/restore/diff panel (*control.Control).
	Snapshots SnapshotStore
}

// Option configures New.
type Option func(*Web)

// WithInspector wires the inspector's components.
func WithInspector(in Inspector) Option { return func(w *Web) { w.insp = in } }

func (i Inspector) controller() (events.Controller, bool) {
	c, ok := i.Events.(events.Controller)
	return c, ok
}

func (w *Web) mountInspector(r chi.Router) {
	r.Get("/ui/inspector", w.inspectorPage)
	r.Get("/ui/inspector/calls", w.inspectorCalls)
	r.Get("/ui/inspector/deliveries", w.inspectorDeliveries)
	r.Post("/ui/inspector/deliveries/{id}/redeliver", w.inspectorRedeliver)
	r.Get("/ui/inspector/apps", w.inspectorApps)
	r.Post("/ui/inspector/apps/{id}/deliver_to", w.inspectorDeliverTo)
	r.Get("/ui/inspector/faults", w.inspectorFaults)
	r.Post("/ui/inspector/faults", w.inspectorAddFault)
	r.Post("/ui/inspector/faults/{id}/delete", w.inspectorDeleteFault)
	r.Get("/ui/inspector/controls", w.inspectorControls)
	r.Post("/ui/inspector/config", w.inspectorConfig)
	r.Post("/ui/inspector/reset", w.inspectorReset)
	r.Get("/ui/inspector/snapshots", w.inspectorSnapshots)
	r.Post("/ui/inspector/snapshots", w.inspectorCreateSnapshot)
	r.Post("/ui/inspector/snapshots/{name}/restore", w.inspectorRestoreSnapshot)
	r.Get("/ui/inspector/diff", w.inspectorDiff)
}

func (w *Web) inspectorPage(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector", nil)
}

func (w *Web) inspectorCalls(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-calls", w.callsView(r))
}

func (w *Web) callsView(r *http.Request) callsView {
	if w.insp.Calls == nil {
		return callsView{Missing: true}
	}
	v := r.URL.Query()
	q := calllog.Query{Method: strings.TrimSpace(v.Get("method")), Channel: strings.TrimSpace(v.Get("channel"))}
	if ok, err := strconv.ParseBool(v.Get("ok")); err == nil {
		q.OK = &ok
	}
	return newCallsView(recentCalls(r.Context(), w.insp.Calls, q, inspectorLimit))
}

// recentCalls returns the newest n matching records, oldest first, paging past the query cap.
func recentCalls(ctx context.Context, src CallSource, q calllog.Query, n int) []calllog.CallRecord {
	var out []calllog.CallRecord
	for {
		res := src.Query(ctx, q)
		out = append(out, res.Calls...)
		if len(out) > n {
			out = out[len(out)-n:]
		}
		if !res.Truncated {
			return out
		}
		q.Since = res.NextSince
	}
}

func (w *Web) deliveriesView(r *http.Request, msg string) deliveriesView {
	ctl, ok := w.insp.controller()
	if !ok {
		return deliveriesView{Missing: true}
	}
	v := r.URL.Query()
	f := events.Filter{EventType: strings.TrimSpace(v.Get("event_type")), State: v.Get("state"), Limit: events.MaxLimit}
	items, _, _ := ctl.Deliveries(f)
	return newDeliveriesView(items, strings.TrimSpace(v.Get("event_id")), inspectorLimit, msg)
}

func (w *Web) inspectorDeliveries(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-deliveries", w.deliveriesView(r, ""))
}

func (w *Web) inspectorRedeliver(rw http.ResponseWriter, r *http.Request) {
	ctl, ok := w.insp.controller()
	if !ok {
		w.render(rw, r, "inspector-deliveries", deliveriesView{Missing: true})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	_, err := ctl.RedeliverDelivery(r.Context(), chi.URLParam(r, "id"), events.RedeliverRequest{Signature: r.PostForm.Get("signature")})
	msg := ""
	if err != nil {
		msg = "redeliver failed: " + err.Error()
	}
	w.render(rw, r, "inspector-deliveries", w.deliveriesView(r, msg))
}

func (w *Web) appsView(msg string) appsView {
	ctl, _ := w.insp.controller()
	return newAppsView(w.svc.Apps(), ctl, msg)
}

func (w *Web) inspectorApps(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-apps", w.appsView(""))
}

func (w *Web) inspectorDeliverTo(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	to := strings.TrimSpace(r.PostForm.Get("deliver_to"))
	msg := ""
	switch {
	case to != "" && !events.ValidDeliverTo(to):
		msg = "deliver_to must be an http(s) URL such as http://localhost:3000"
	default:
		if _, err := w.svc.SetAppDeliverTo(r.Context(), chi.URLParam(r, "id"), to); err != nil {
			msg = "set deliver_to: " + err.Error()
		}
	}
	w.render(rw, r, "inspector-apps", w.appsView(msg))
}

func (w *Web) faultsView(msg string) faultsView {
	if w.insp.Faults == nil {
		return faultsView{Missing: true}
	}
	return newFaultsView(w.insp.Faults.List(), msg)
}

func (w *Web) inspectorFaults(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-faults", w.faultsView(""))
}

func (w *Web) inspectorAddFault(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Faults == nil {
		w.render(rw, r, "inspector-faults", faultsView{Missing: true})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	spec, err := faultSpec(r)
	if err == nil {
		_, err = w.insp.Faults.Add(spec)
	}
	msg := ""
	if err != nil {
		msg = faultMessage(err)
	}
	w.render(rw, r, "inspector-faults", w.faultsView(msg))
}

func (w *Web) inspectorDeleteFault(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Faults == nil {
		w.render(rw, r, "inspector-faults", faultsView{Missing: true})
		return
	}
	msg := ""
	if id := chi.URLParam(r, "id"); !w.insp.Faults.Delete(id) {
		msg = "no fault " + id
	}
	w.render(rw, r, "inspector-faults", w.faultsView(msg))
}

// faultSpec reads the add-fault form. Blank numbers mean "unset".
func faultSpec(r *http.Request) (faults.Spec, error) {
	f := r.PostForm
	var errs []error
	num := func(key string) int {
		s := strings.TrimSpace(f.Get(key))
		if s == "" {
			return 0
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a whole number", key))
		}
		return n
	}
	spec := faults.Spec{
		Method: strings.TrimSpace(f.Get("method")),
		Match:  faults.Match{Channel: strings.TrimSpace(f.Get("channel")), AppID: strings.TrimSpace(f.Get("app_id"))},
		Nth:    num("nth"), Every: num("every"), Times: num("times"), Once: f.Get("once") != "",
		Action: faults.Action{
			Status: num("status"), Error: strings.TrimSpace(f.Get("error")),
			RetryAfter: num("retry_after"), DelayMS: num("delay_ms"),
		},
	}
	return spec, errors.Join(errs...)
}

func faultMessage(err error) string {
	var ve *faults.ValidationError
	if !errors.As(err, &ve) {
		return err.Error()
	}
	parts := make([]string, 0, len(ve.Details))
	for _, d := range ve.Details {
		parts = append(parts, strings.TrimSpace(d.Pointer+" "+d.Message))
	}
	return "invalid fault: " + strings.Join(parts, "; ")
}

func (w *Web) controlsView(msg string) controlsView {
	ctl, ok := w.insp.controller()
	if !ok {
		return controlsView{Missing: true, Error: msg}
	}
	var dropped int64
	if d, ok := w.insp.Events.(interface{ Dropped() int64 }); ok {
		dropped = d.Dropped()
	}
	return newControlsView(ctl.Config(), dropped, msg)
}

func (w *Web) inspectorControls(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-controls", w.controlsView(""))
}

// inspectorConfig patches the global delivery config with whichever fields were sent.
func (w *Web) inspectorConfig(rw http.ResponseWriter, r *http.Request) {
	ctl, ok := w.insp.controller()
	if !ok {
		w.render(rw, r, "inspector-controls", controlsView{Missing: true})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	patch, err := configPatch(r)
	if err == nil {
		_, err = ctl.SetConfig(patch)
	}
	msg := ""
	if err != nil {
		msg = "delivery config: " + err.Error()
	}
	w.render(rw, r, "inspector-controls", w.controlsView(msg))
}

func configPatch(r *http.Request) (events.ConfigPatch, error) {
	var p events.ConfigPatch
	if s := r.PostForm.Get("signature"); s != "" {
		p.Signature = &s
	}
	if s := r.PostForm.Get("paused"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return p, errors.New("paused must be true or false")
		}
		p.Paused = &b
	}
	if s := strings.TrimSpace(r.PostForm.Get("timeout_ms")); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return p, errors.New("timeout_ms must be a whole number")
		}
		p.TimeoutMS = &n
	}
	return p, nil
}

func (w *Web) inspectorReset(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Control == nil {
		w.render(rw, r, "inspector-reset", resetView{Error: "reset is not available on this server"})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	all, _ := strconv.ParseBool(r.PostForm.Get("all"))
	res, err := w.insp.Control.Reset(r.Context(), control.ResetOpts{All: all, Seed: strings.TrimSpace(r.PostForm.Get("seed"))})
	switch {
	case errors.Is(err, control.ErrResetInProgress):
		w.render(rw, r, "inspector-reset", resetView{Error: "another reset is running; retry when it finishes"})
	case errors.Is(err, service.ErrResetIDCollision):
		w.render(rw, r, "inspector-reset", resetView{Error: "that seed collides with an app bot user; pick another seed (nothing changed)"})
	case err != nil:
		w.fail(rw, r, err)
	default:
		w.render(rw, r, "inspector-reset", newResetView(res))
	}
}
