package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/appmanifest"
	"localslack/internal/events"
	"localslack/internal/service"
)

const maxManifestBytes = 1 << 20

func mountApps(r chi.Router, d Deps) {
	if d.Svc == nil {
		return
	}
	a := appsAPI{Deps: d}
	r.Post("/_localslack/apps", a.create)
	r.Get("/_localslack/apps", a.list)
	r.Get("/_localslack/apps/{id}", a.get)
	r.Patch("/_localslack/apps/{id}", a.patch)
	r.Delete("/_localslack/apps/{id}", a.remove)
	r.Post("/_localslack/apps/{id}/verify", a.verify)
}

type appsAPI struct{ Deps }

// controller is the dispatcher's control surface, if it has one.
func (d Deps) controller() (events.Controller, bool) {
	c, ok := d.Events.(events.Controller)
	return c, ok
}

// problem writes the /_localslack error shape with optional details.
func (d Deps) problem(w http.ResponseWriter, r *http.Request, status int, code, msg string, details []map[string]string) {
	e := map[string]any{"code": code, "message": msg}
	if len(details) > 0 {
		e["details"] = details
	}
	writeJSON(r.Context(), d.Log, w, status, map[string]any{"error": e})
}

func (d Deps) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	d.Log.ErrorContext(r.Context(), what, slog.Any("error", err))
	d.problem(w, r, http.StatusInternalServerError, "internal_error", "internal error", nil)
}

// createRequest is the POST /apps body once decoded.
type createRequest struct {
	parsed        appmanifest.Parsed
	appID         string
	signingSecret string
	botToken      string
	appToken      string
	deliverTo     string
	verify        bool
}

func (a appsAPI) create(w http.ResponseWriter, r *http.Request) {
	req, ok := a.decodeCreate(w, r)
	if !ok {
		return
	}
	app, err := a.Svc.RegisterApp(r.Context(), service.RegisterInput{
		Manifest: req.parsed, AppID: req.appID, SigningSecret: req.signingSecret,
		BotToken: req.botToken, AppToken: req.appToken, DeliverTo: req.deliverTo,
	})
	switch {
	case errors.Is(err, service.ErrAppExists):
		a.problem(w, r, http.StatusConflict, "app_exists", "an app with this id already exists; delete it first or use another app_id", nil)
		return
	case errors.Is(err, service.ErrTokenInUse):
		a.problem(w, r, http.StatusConflict, "already_exists", err.Error()+"; bot_token and app_token must be unique across apps", nil)
		return
	case err != nil:
		a.internal(w, r, "register app", err)
		return
	}
	if ctl, ok := a.controller(); ok {
		if t := ctl.Resolve(app); t.Blocked {
			a.Log.WarnContext(r.Context(), "app events are blocked", slog.String("app_id", app.ID), slog.String("reason", t.Reason),
				slog.String("fix", "localslack apps set-url "+app.ID+" http://localhost:3000"))
		} else if req.verify && !app.SocketMode {
			if _, err := ctl.Verify(r.Context(), app.ID); err != nil {
				a.internal(w, r, "verify app", err)
				return
			}
		}
	}
	a.respondApp(w, r, http.StatusCreated, app.ID, "/_localslack/apps/"+app.ID)
}

// decodeCreate reads a raw manifest (JSON or YAML) or the {"manifest": ...} envelope.
func (a appsAPI) decodeCreate(w http.ResponseWriter, r *http.Request) (createRequest, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxManifestBytes))
	if err != nil {
		a.problem(w, r, http.StatusBadRequest, "malformed_request", "could not read request body", nil)
		return createRequest{}, false
	}
	req := createRequest{verify: true}
	manifestBytes := body
	var env struct {
		Manifest      json.RawMessage `json:"manifest"`
		DeliverTo     string          `json:"deliver_to"`
		SigningSecret string          `json:"signing_secret"`
		BotToken      string          `json:"bot_token"`
		AppToken      string          `json:"app_token"`
		AppID         string          `json:"app_id"`
		Verify        *bool           `json:"verify"`
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(body, &probe) == nil && probe["manifest"] != nil && probe["display_information"] == nil {
		if err := json.Unmarshal(body, &env); err != nil {
			a.problem(w, r, http.StatusBadRequest, "malformed_request", "invalid request body: "+err.Error(), nil)
			return createRequest{}, false
		}
		manifestBytes = env.Manifest
		var asString string
		if json.Unmarshal(env.Manifest, &asString) == nil {
			manifestBytes = []byte(asString) // a YAML (or JSON) document inside a string
		}
		req.appID, req.signingSecret, req.botToken, req.appToken, req.deliverTo = env.AppID, env.SigningSecret, env.BotToken, env.AppToken, env.DeliverTo
		if env.Verify != nil {
			req.verify = *env.Verify
		}
	}
	parsed, err := appmanifest.Parse(manifestBytes)
	var verr *appmanifest.ValidationError
	switch {
	case errors.As(err, &verr):
		a.problem(w, r, http.StatusUnprocessableEntity, "invalid_manifest", verr.Error(), problemDetails(verr))
		return createRequest{}, false
	case err != nil:
		a.problem(w, r, http.StatusBadRequest, "malformed_request", err.Error(), nil)
		return createRequest{}, false
	}
	if req.deliverTo != "" && !events.ValidDeliverTo(req.deliverTo) {
		a.invalidDeliverTo(w, r)
		return createRequest{}, false
	}
	req.parsed = parsed
	return req, true
}

func problemDetails(e *appmanifest.ValidationError) []map[string]string {
	out := make([]map[string]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		out = append(out, map[string]string{"message": p.Message, "pointer": p.Pointer})
	}
	return out
}

func (a appsAPI) invalidDeliverTo(w http.ResponseWriter, r *http.Request) {
	a.problem(w, r, http.StatusUnprocessableEntity, "invalid_deliver_to",
		"deliver_to must be an http(s) URL such as http://localhost:3000",
		[]map[string]string{{"message": "must be an http(s) URL", "pointer": "/deliver_to"}})
}

func (a appsAPI) list(w http.ResponseWriter, r *http.Request) {
	apps := a.Svc.Apps()
	items := make([]appView, 0, len(apps))
	ctl, _ := a.controller()
	for _, app := range apps {
		items = append(items, newAppView(app, ctl))
	}
	writeJSON(r.Context(), a.Log, w, http.StatusOK, map[string]any{"items": items})
}

func (a appsAPI) get(w http.ResponseWriter, r *http.Request) {
	a.respondApp(w, r, http.StatusOK, chi.URLParam(r, "id"), "")
}

// respondApp writes the current view of app id (404 if it is gone).
func (a appsAPI) respondApp(w http.ResponseWriter, r *http.Request, status int, id, location string) {
	app, err := a.Svc.App(r.Context(), id)
	if errors.Is(err, service.ErrAppNotFound) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.internal(w, r, "load app", err)
		return
	}
	ctl, _ := a.controller()
	if location != "" {
		w.Header().Set("Location", location)
	}
	writeJSON(r.Context(), a.Log, w, status, newAppView(app, ctl))
}

func (d Deps) notFound(w http.ResponseWriter, r *http.Request) {
	d.problem(w, r, http.StatusNotFound, "app_not_found", "no such app", nil)
}

func (a appsAPI) patch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		DeliverTo *string `json:"deliver_to"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxManifestBytes)).Decode(&body); err != nil {
		a.problem(w, r, http.StatusBadRequest, "malformed_request", "invalid request body: "+err.Error(), nil)
		return
	}
	if body.DeliverTo != nil {
		if *body.DeliverTo != "" && !events.ValidDeliverTo(*body.DeliverTo) {
			a.invalidDeliverTo(w, r)
			return
		}
		if _, err := a.Svc.SetAppDeliverTo(r.Context(), id, *body.DeliverTo); err != nil {
			a.appError(w, r, "set deliver_to", err)
			return
		}
	}
	a.respondApp(w, r, http.StatusOK, id, "")
}

func (a appsAPI) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.Svc.DeleteApp(r.Context(), id); err != nil {
		a.appError(w, r, "delete app", err)
		return
	}
	if ctl, ok := a.controller(); ok {
		ctl.ClearAppConfig(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a appsAPI) verify(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctl, ok := a.controller()
	if !ok {
		a.problem(w, r, http.StatusNotImplemented, "not_implemented", "this server has no event dispatcher", nil)
		return
	}
	if _, err := ctl.Verify(r.Context(), id); err != nil {
		if errors.Is(err, events.ErrAppNotFound) {
			a.notFound(w, r)
			return
		}
		a.internal(w, r, "verify app", err)
		return
	}
	a.respondApp(w, r, http.StatusOK, id, "")
}

func (a appsAPI) appError(w http.ResponseWriter, r *http.Request, what string, err error) {
	if errors.Is(err, service.ErrAppNotFound) {
		a.notFound(w, r)
		return
	}
	a.internal(w, r, what, err)
}
