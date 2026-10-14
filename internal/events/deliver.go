package events

import (
	"context"
	"log/slog"
	"math/rand/v2"

	"localslack/internal/service"
)

// outgoing is one event addressed to one app, before it is recorded.
type outgoing struct {
	app                service.App
	eventID, eventType string
	payload            []byte
	injected           bool
	sigMode            string // override; empty = the delivery config's
	retryNum           int    // header on the first attempt; later attempts count up from it
	retryReason        string
}

// job is a recorded delivery ready to run.
type job struct {
	id, appID, secret string
	tr                Transport
	payload           []byte
	spec              Spec
	cfg               DeliveryConfig
	baseRetry         int
	baseReason        string
}

// enqueue records o in the trace and starts (or holds) its delivery. It does
// nothing when gen is stale (a Reset happened) or the engine is closed.
func (e *Engine) enqueue(gen int64, o outgoing) (Delivery, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.gen.Load() != gen {
		return Delivery{}, false
	}
	cfg := e.configLocked(o.app.ID)
	d := Delivery{
		EventID: o.eventID, EventType: o.eventType, AppID: o.app.ID, Transport: "http",
		State: StatePending, Injected: o.injected, Payload: o.payload,
	}
	j := &job{
		appID: o.app.ID, secret: o.app.SigningSecret, tr: e.tr, payload: o.payload, cfg: cfg,
		baseRetry: o.retryNum, baseReason: o.retryReason,
	}
	j.spec = Spec{Secret: o.app.SigningSecret, Mode: cfg.Signature, StaleSeconds: cfg.StaleSeconds}
	if o.sigMode != "" {
		j.spec.Mode = o.sigMode
	}
	failure := e.routeJobLocked(&d, j, o.app)
	stored := e.trace.Add(d)
	e.sent.put(o.eventID, storedDelivery{appID: o.app.ID, typ: o.eventType, payload: o.payload, injected: o.injected})
	if failure != "" {
		e.failLocked(stored.ID, failure)
		return stored, true
	}
	j.id = stored.ID
	if e.cfg.Paused || cfg.Paused {
		e.held = append(e.held, j)
		return stored, true
	}
	e.startLocked(j)
	return stored, true
}

// routeJobLocked picks the transport and URL; a non-empty result is why delivery is impossible.
func (e *Engine) routeJobLocked(d *Delivery, j *job, app service.App) string {
	if app.SocketMode {
		d.Transport = "socket"
		if e.sock == nil {
			return "socket mode delivery is not available"
		}
		j.tr = e.sock
		return ""
	}
	t := e.policy.Resolve(app.RequestURL, app.DeliverTo)
	if t.Blocked {
		if !e.warned[app.ID] {
			e.warned[app.ID] = true
			e.log.Warn("event delivery blocked; fix with the command below",
				slog.String("app_id", app.ID), slog.String("reason", t.Reason),
				slog.String("fix", "localslack apps set-url "+app.ID+" http://localhost:3000"))
		}
		return "delivery blocked: " + t.Reason
	}
	j.spec.URL = t.URL
	return ""
}

func (e *Engine) failLocked(id, msg string) {
	at := e.clock.Now()
	e.trace.Update(id, func(d *Delivery) {
		d.State = StateFailed
		d.Attempts = append(d.Attempts, Attempt{N: 1, At: at, Error: msg})
	})
}

// lane is one app's FIFO of first attempts. Its drain goroutine runs them one
// at a time, so an app sees its events in publish order and never concurrently.
// Retries leave the lane (see retry): their timers are independent, so a retry
// may overtake or trail later events.
type lane struct {
	queue   []laneItem
	running bool
}

type laneItem struct {
	ctx context.Context
	j   *job
}

func (e *Engine) startLocked(j *job) {
	ln := e.lanes[j.appID]
	if ln == nil {
		ln = &lane{}
		e.lanes[j.appID] = ln
	}
	ln.queue = append(ln.queue, laneItem{ctx: e.ctx, j: j})
	if !ln.running {
		ln.running = true
		e.wg.Add(1)
		go e.drain(ln)
	}
}

// drain runs ln's first attempts in order until the lane is empty.
func (e *Engine) drain(ln *lane) {
	defer e.wg.Done()
	for {
		e.mu.Lock()
		if len(ln.queue) == 0 {
			ln.running = false
			e.mu.Unlock()
			return
		}
		it := ln.queue[0]
		ln.queue = ln.queue[1:]
		e.mu.Unlock()
		if it.ctx.Err() != nil {
			continue // reset or shutdown while queued
		}
		if again, reason := e.attempt(it.ctx, it.j, 0, it.j.baseReason); again {
			e.startRetry(it.ctx, it.j, 1, reason)
		}
	}
}

func (e *Engine) startRetry(ctx context.Context, j *job, n int, reason string) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.retry(ctx, j, n, reason)
	}()
}

// retry waits out each configured delay and tries again until delivered or out of retries.
func (e *Engine) retry(ctx context.Context, j *job, n int, reason string) {
	delays := j.cfg.RetryDelays()
	for ; ; n++ {
		select {
		case <-e.clock.After(delays[n-1]):
		case <-ctx.Done():
			return
		}
		again, next := e.attempt(ctx, j, n, reason)
		if !again {
			return
		}
		reason = next
	}
}

// flushHeldLocked starts held deliveries whose app is no longer paused.
func (e *Engine) flushHeldLocked() {
	var keep []*job
	for _, j := range e.held {
		if e.cfg.Paused || e.configLocked(j.appID).Paused {
			keep = append(keep, j)
			continue
		}
		e.startLocked(j)
	}
	e.held = keep
}

// attempt makes attempt number n (0-based) at j and records it. It reports
// whether another attempt follows and the retry reason to send with it.
func (e *Engine) attempt(ctx context.Context, j *job, n int, reason string) (again bool, nextReason string) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return false, ""
	}
	spec := j.spec
	spec.RetryNum, spec.RetryReason = j.baseRetry+n, reason
	actx, cancel := context.WithTimeout(ctx, j.cfg.Timeout())
	att := j.tr.Deliver(actx, Delivery{AppID: j.appID, Payload: j.payload, Spec: spec})
	cancel()
	if ctx.Err() != nil {
		return false, "" // reset or shutdown: a late result must not be recorded
	}
	att.N, att.At = n+1, e.clock.Now()
	final := att.OK() || att.NoRetry || n >= len(j.cfg.RetryDelays())
	state := StateRetrying
	switch {
	case att.OK():
		state = StateDelivered
	case final:
		state = StateFailed
	}
	e.trace.Update(j.id, func(d *Delivery) {
		d.Attempts = append(d.Attempts, att)
		d.State = state
	})
	return !final, att.FailReason
}

const idAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString returns n random characters from idAlphabet (ids, not secrets).
func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = idAlphabet[rand.IntN(len(idAlphabet))]
	}
	return string(b)
}

// newEventID returns a Slack-style event id ("Ev" + 10 characters).
func newEventID() string { return "Ev" + randomString(10) }

// storedDelivery is what redelivery needs to resend one app's event.
type storedDelivery struct {
	appID, typ string
	payload    []byte
	injected   bool
}

// eventStore remembers the last cap events, keyed by event id (unique per app delivery).
type eventStore struct {
	cap   int
	order []string
	m     map[string]storedDelivery
}

func newEventStore(capacity int) *eventStore {
	return &eventStore{cap: capacity, m: map[string]storedDelivery{}}
}

func (s *eventStore) put(eventID string, sd storedDelivery) {
	if _, seen := s.m[eventID]; !seen {
		s.order = append(s.order, eventID)
		if len(s.order) > s.cap {
			delete(s.m, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.m[eventID] = sd
}

func (s *eventStore) get(eventID string) (storedDelivery, bool) {
	sd, ok := s.m[eventID]
	return sd, ok
}
