package events

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"localslack/internal/service"
)

// Errors returned by the control operations.
var (
	ErrAppNotFound   = errors.New("events: app not found")
	ErrEventNotFound = errors.New("events: event not found")
	// ErrDeliveryNotFound means the delivery id is unknown or aged out of the trace.
	ErrDeliveryNotFound = errors.New("events: delivery not found")
	ErrInvalidEvent     = errors.New("events: invalid event")
)

// Controller is what the admin API drives; *Engine implements it. admin
// type-asserts a Dispatcher to this, so a Noop dispatcher just has no controls.
type Controller interface {
	Dispatcher
	Resolve(app service.App) Target
	Verify(ctx context.Context, appID string) (VerifyResult, error)
	VerifyError(appID string) string
	Deliveries(f Filter) (items []Delivery, next, generation int64)
	WaitDeliveries(ctx context.Context, f Filter, minCount int, timeout time.Duration) (items []Delivery, next, generation int64)
	Delivery(id string) (Delivery, bool)
	ClearDeliveries()
	Inject(ctx context.Context, r InjectRequest) (InjectResult, error)
	Redeliver(ctx context.Context, eventID string, r RedeliverRequest) (InjectResult, error)
	RedeliverDelivery(ctx context.Context, deliveryID string, r RedeliverRequest) (InjectResult, error)
	Config() DeliveryConfig
	// SetConfig merges p into the global config (PATCH); ReplaceConfig is PUT:
	// the process defaults with p applied, so omitted fields reset.
	SetConfig(p ConfigPatch) (DeliveryConfig, error)
	ReplaceConfig(p ConfigPatch) (DeliveryConfig, error)
	// AppConfig is the effective config (global + override, inherited live) and the sparse override.
	AppConfig(appID string) (effective DeliveryConfig, override ConfigPatch)
	SetAppConfig(appID string, p ConfigPatch) (DeliveryConfig, error)
	ReplaceAppConfig(appID string, p ConfigPatch) (DeliveryConfig, error)
	ClearAppConfig(appID string)
}

// VerifyResult is the outcome of a url_verification handshake.
type VerifyResult struct {
	Verified bool   `json:"verified"`
	Error    string `json:"error,omitempty"`
}

// VerifyError is why app's last verification failed ("" if it passed or never ran).
func (e *Engine) VerifyError(appID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.verifyErr[appID]
}

func (e *Engine) app(id string) (service.App, bool) {
	for _, a := range e.src.Apps() {
		if a.ID == id {
			return a, true
		}
	}
	return service.App{}, false
}

// Verify sends a signed url_verification challenge to the app and records
// whether it echoed it. Failure is not fatal: events are still delivered.
func (e *Engine) Verify(ctx context.Context, appID string) (VerifyResult, error) {
	app, ok := e.app(appID)
	if !ok {
		return VerifyResult{}, ErrAppNotFound
	}
	res := e.verify(ctx, app)
	e.mu.Lock()
	e.verifyErr[appID] = res.Error
	e.mu.Unlock()
	if err := e.src.SetAppVerified(ctx, appID, res.Verified); err != nil {
		e.log.Error("store verified flag", slog.String("app_id", appID), slog.Any("error", err))
	}
	if !res.Verified {
		e.log.Warn("url_verification failed; events are still delivered",
			slog.String("app_id", appID), slog.String("reason", res.Error))
	}
	return res, nil
}

func (e *Engine) verify(ctx context.Context, app service.App) VerifyResult {
	if app.SocketMode {
		return VerifyResult{Error: "socket mode app: there is no request URL to verify"}
	}
	target := e.policy.Resolve(app.RequestURL, app.DeliverTo)
	if target.Blocked {
		return VerifyResult{Error: "delivery blocked: " + target.Reason}
	}
	challenge := randomString(32)
	body, err := json.Marshal(map[string]string{"token": app.VerificationToken, "challenge": challenge, "type": "url_verification"})
	if err != nil {
		return VerifyResult{Error: err.Error()}
	}
	e.mu.Lock()
	timeout := e.cfg.Timeout()
	e.mu.Unlock()
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	att := e.tr.Deliver(actx, Delivery{Payload: body, Spec: Spec{URL: target.URL, Secret: app.SigningSecret, Mode: SigValid}})
	if !att.OK() {
		return VerifyResult{Error: "request failed: " + att.Error}
	}
	if !echoesChallenge(att.ResponseBody, challenge) {
		return VerifyResult{Error: "app answered HTTP " + strconv.Itoa(att.Status) + " but did not echo the challenge"}
	}
	return VerifyResult{Verified: true}
}

// echoesChallenge accepts Slack's JSON form and the plain-text form (permissive).
func echoesChallenge(body, challenge string) bool {
	body = strings.TrimSpace(body)
	if body == challenge {
		return true
	}
	var j struct {
		Challenge string `json:"challenge"`
	}
	return json.Unmarshal([]byte(body), &j) == nil && j.Challenge == challenge
}

// InjectRequest asks for a hand-made event to be delivered.
type InjectRequest struct {
	Event map[string]any
	// AppID targets one app (whatever it subscribes to); empty means every
	// app subscribed to the event type.
	AppID string
	// Signature overrides the signature mode for these deliveries.
	Signature string
	// Envelope is shallow-merged over the generated event_callback.
	Envelope map[string]any
}

// RedeliverRequest asks for a stored event to be sent again.
type RedeliverRequest struct {
	AppID       string
	RetryNum    int
	RetryReason string
	Signature   string
}

// DeliveryRef points at one queued delivery. Event ids are per app delivery
// (as on real Slack), so every ref carries its own.
type DeliveryRef struct {
	AppID      string `json:"app_id"`
	EventID    string `json:"event_id"`
	DeliveryID string `json:"delivery_id"`
}

// InjectResult is the reply to Inject and Redeliver.
type InjectResult struct {
	Deliveries []DeliveryRef `json:"deliveries"`
}

func validSignature(mode string) bool {
	return mode == "" || slices.Contains([]string{SigValid, SigInvalid, SigStale, SigMissing}, mode)
}

// subscribed reports whether app subscribes to an injected event of type typ.
func subscribed(app service.App, typ string) bool {
	if typ == "message" {
		return slices.ContainsFunc(app.BotEvents, func(s string) bool { return strings.HasPrefix(s, "message.") })
	}
	return slices.Contains(app.BotEvents, typ)
}

// Inject delivers a hand-made event: one delivery per target app, each with its own event_id.
func (e *Engine) Inject(ctx context.Context, r InjectRequest) (InjectResult, error) {
	typ, _ := r.Event["type"].(string)
	if typ == "" || !validSignature(r.Signature) {
		return InjectResult{}, ErrInvalidEvent
	}
	apps := e.src.Apps()
	if r.AppID != "" {
		app, ok := e.app(r.AppID)
		if !ok {
			return InjectResult{}, ErrAppNotFound
		}
		apps = []service.App{app}
	} else {
		apps = slices.DeleteFunc(slices.Clone(apps), func(a service.App) bool { return !subscribed(a, typ) })
	}
	team, ok := e.team()
	if !ok {
		return InjectResult{}, errors.New("events: workspace unavailable")
	}
	res := InjectResult{Deliveries: []DeliveryRef{}}
	gen := e.gen.Load()
	for _, app := range apps {
		eventID := newEventID()
		body, err := Envelope(app, team, eventID, e.clock.Now(), r.Event)
		if err == nil && len(r.Envelope) > 0 {
			body, err = MergeEnvelope(body, r.Envelope)
		}
		if err != nil {
			return InjectResult{}, err
		}
		d, ok := e.enqueue(gen, outgoing{app: app, eventID: eventID, eventType: typ, payload: body, injected: true, sigMode: r.Signature})
		if ok {
			res.Deliveries = append(res.Deliveries, DeliveryRef{AppID: app.ID, EventID: eventID, DeliveryID: d.ID})
		}
	}
	return res, nil
}

// Redeliver sends the event with this event_id again, unchanged, as Slack does
// for a retry. Event ids are per app delivery, so the id alone is unambiguous;
// r.AppID is optional and, when set, must be the event's app (else ErrEventNotFound).
func (e *Engine) Redeliver(ctx context.Context, eventID string, r RedeliverRequest) (InjectResult, error) {
	if !validSignature(r.Signature) {
		return InjectResult{}, ErrInvalidEvent
	}
	e.mu.Lock()
	stored, ok := e.sent.get(eventID)
	e.mu.Unlock()
	if !ok || (r.AppID != "" && stored.appID != r.AppID) {
		return InjectResult{}, ErrEventNotFound
	}
	return e.resend(ctx, eventID, stored, r)
}

// RedeliverDelivery is Redeliver addressed by delivery id (the trace record).
func (e *Engine) RedeliverDelivery(ctx context.Context, deliveryID string, r RedeliverRequest) (InjectResult, error) {
	d, ok := e.trace.Get(deliveryID)
	if !ok || (r.AppID != "" && r.AppID != d.AppID) {
		return InjectResult{}, ErrDeliveryNotFound
	}
	r.AppID = ""
	return e.Redeliver(ctx, d.EventID, r)
}

func (e *Engine) resend(_ context.Context, eventID string, s storedDelivery, r RedeliverRequest) (InjectResult, error) {
	if r.RetryNum <= 0 {
		r.RetryNum = 1
	}
	if r.RetryReason == "" {
		r.RetryReason = ReasonTimeout
	}
	app, ok := e.app(s.appID)
	if !ok {
		return InjectResult{}, ErrAppNotFound
	}
	res := InjectResult{Deliveries: []DeliveryRef{}}
	d, ok := e.enqueue(e.gen.Load(), outgoing{
		app: app, eventID: eventID, eventType: s.typ, payload: s.payload, injected: s.injected,
		sigMode: r.Signature, retryNum: r.RetryNum, retryReason: r.RetryReason,
	})
	if ok {
		res.Deliveries = append(res.Deliveries, DeliveryRef{AppID: app.ID, EventID: eventID, DeliveryID: d.ID})
	}
	return res, nil
}

// Config is the global delivery config.
func (e *Engine) Config() DeliveryConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

// SetConfig merges p into the global config (PATCH); unpausing releases held events.
func (e *Engine) SetConfig(p ConfigPatch) (DeliveryConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setGlobalLocked(e.cfg, p)
}

// ReplaceConfig replaces the global config (PUT): the process defaults with p
// applied, so fields p omits go back to their defaults.
func (e *Engine) ReplaceConfig(p ConfigPatch) (DeliveryConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setGlobalLocked(e.defaults, p)
}

func (e *Engine) setGlobalLocked(base DeliveryConfig, p ConfigPatch) (DeliveryConfig, error) {
	next, err := base.Apply(p)
	if err != nil {
		return e.cfg, err
	}
	e.cfg = next
	e.flushHeldLocked()
	return next, nil
}

// AppConfig is app's effective config (global with the app's sparse override on
// top, so unset fields follow later global changes) and the override itself.
func (e *Engine) AppConfig(appID string) (effective DeliveryConfig, override ConfigPatch) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.configLocked(appID), e.overrides[appID]
}

// SetAppConfig merges p into app's sparse override (PATCH).
func (e *Engine) SetAppConfig(appID string, p ConfigPatch) (DeliveryConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setOverrideLocked(appID, e.overrides[appID].Merge(p))
}

// ReplaceAppConfig replaces app's override with exactly p (PUT): fields p omits inherit.
func (e *Engine) ReplaceAppConfig(appID string, p ConfigPatch) (DeliveryConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setOverrideLocked(appID, p)
}

func (e *Engine) setOverrideLocked(appID string, o ConfigPatch) (DeliveryConfig, error) {
	eff, err := e.cfg.Apply(o) // validates the override against the current global
	if err != nil {
		return e.configLocked(appID), err
	}
	if o.IsZero() {
		delete(e.overrides, appID)
	} else {
		e.overrides[appID] = o
	}
	e.flushHeldLocked()
	return eff, nil
}

// ClearAppConfig drops app's override.
func (e *Engine) ClearAppConfig(appID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.overrides, appID)
	e.flushHeldLocked()
}
