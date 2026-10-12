package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"localslack/internal/calllog"
	"localslack/internal/faults"
	"localslack/internal/service"
)

// serve runs one request through the stages, all inside the Recorder:
// resolve -> parse -> identify -> intercept -> enforce auth -> call -> respond.
func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	start := a.now()
	rw := newCapture(w)
	st := &state{method: chi.URLParam(r, "method")}
	a.run(rw, r, st)
	a.rec.Record(st.record(r, rw, start, a.now()))
}

// state is what the stages learn about a call, for the record.
type state struct {
	method         string
	params         url.Values
	raw            map[string]any
	p              service.Principal
	faultID        string
	notImplemented bool
}

func (a *API) run(w http.ResponseWriter, r *http.Request, st *state) {
	ctx := r.Context()
	sp, ok := a.methods[st.method]
	if !ok {
		st.notImplemented = true
		a.fallback(w, r)
		return
	}
	params, raw, err := parseParams(r)
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	st.params, st.raw = params, raw
	token := bearer(r, params)
	st.p = a.svc.Identify(ctx, token)
	if a.intercept(ctx, w, r, st) {
		return
	}
	p, err := a.enforce(ctx, sp.auth, token, st.p)
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	st.p = p
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	out, err := sp.fn(ctx, &call{p: p, params: params, baseURL: scheme + "://" + r.Host + "/"})
	if err != nil {
		a.fail(ctx, w, err)
		return
	}
	out["ok"] = true
	a.write(ctx, w, http.StatusOK, out)
}

// intercept applies a fired fault; true means the response is already written.
func (a *API) intercept(ctx context.Context, w http.ResponseWriter, r *http.Request, st *state) bool {
	ov, fired := a.icpt.Intercept(ctx, faults.Request{
		Method: st.method, HTTPMethod: r.Method, Params: st.params, Raw: st.raw,
		UserID: st.p.User.ID, AppID: appID(st.p), TokenKind: tokenKind(st.p),
	})
	if !fired {
		return false
	}
	st.faultID = ov.FaultID
	if ov.Delay > 0 {
		t := time.NewTimer(ov.Delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return true
		}
	}
	if !ov.Respond {
		return false
	}
	for k, v := range ov.Headers {
		w.Header()[k] = v
	}
	a.write(ctx, w, ov.Status, ov.Body)
	return true
}

// enforce checks the credential the method's spec requires.
func (a *API) enforce(ctx context.Context, mode authMode, token string, p service.Principal) (service.Principal, error) {
	switch mode {
	case authNone:
		return p, nil
	case authApp:
		if p.TokenKind == service.TokenApp && p.App != nil {
			return p, nil
		}
	case authToken:
		if p.User.ID != "" {
			return p, nil
		}
	}
	// Re-resolve for the precise error (and to surface real store faults).
	if _, err := a.svc.Authenticate(ctx, token); err != nil {
		return p, err
	}
	return p, service.ErrInvalidAuth
}

func appID(p service.Principal) string {
	if p.App == nil {
		return ""
	}
	return p.App.ID
}

func tokenKind(p service.Principal) string {
	if p.TokenKind == service.TokenNone {
		return "none"
	}
	return string(p.TokenKind)
}

func (st *state) record(r *http.Request, rw *capture, start, end time.Time) calllog.CallRecord {
	rec := calllog.CallRecord{
		Time: start, RequestID: middleware.GetReqID(r.Context()), Method: st.method, HTTPMethod: r.Method,
		Status: rw.status, Params: paramMap(st.params, st.raw), FaultID: st.faultID, NotImplemented: st.notImplemented,
		Actor:      calllog.Actor{UserID: st.p.User.ID, AppID: appID(st.p), TokenKind: tokenKind(st.p)},
		DurationMS: end.Sub(start).Milliseconds(),
		Response:   rw.body(),
	}
	rec.OK, _ = rec.Response["ok"].(bool)
	rec.Error, _ = rec.Response["error"].(string)
	return rec
}

// paramMap prefers the raw JSON body (types intact) over the flattened values.
func paramMap(v url.Values, raw map[string]any) map[string]any {
	out := make(map[string]any, len(v))
	for k, vs := range v {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	for k, x := range raw {
		out[k] = x
	}
	return out
}
