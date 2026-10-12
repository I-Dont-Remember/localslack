package events

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"localslack/internal/service"
)

// Source is what the engine needs to know about apps and the workspace;
// *service.Service implements it.
type Source interface {
	Apps() []service.App
	Team(ctx context.Context) (service.Team, error)
	SetAppVerified(ctx context.Context, id string, verified bool) error
}

// Options configure New. Zero values get sane defaults.
type Options struct {
	Source Source
	// Transport sends HTTP deliveries (default: NewHTTPTransport(Clock)).
	Transport Transport
	// Socket sends deliveries to Socket Mode apps; nil records them as failed.
	Socket Transport
	Clock  Clock
	Policy URLPolicy
	// Config is the global delivery config (default: 3s timeout, no retries).
	Config               DeliveryConfig
	TraceSize, QueueSize int
	Log                  *slog.Logger
}

const (
	defaultTraceSize = 2000
	defaultQueueSize = 1024
	maxInFlight      = 64
)

// Engine is the Dispatcher: it fans service events out to apps, delivers them
// with signing, timeouts and retries, and records every attempt in a Trace.
type Engine struct {
	src       Source
	tr, sock  Transport
	clock     Clock
	policy    URLPolicy
	log       *slog.Logger
	defaults  DeliveryConfig
	trace     *Trace
	queue     chan queued
	sem       chan struct{}
	root      context.Context
	rootStop  context.CancelFunc
	loopDone  chan struct{}
	wg        sync.WaitGroup
	dropped   atomic.Int64
	paused    atomic.Bool
	gen       atomic.Int64
	closeOnce sync.Once

	mu        sync.Mutex // guards everything below
	closed    bool
	ctx       context.Context // current generation's context
	cancel    context.CancelFunc
	cfg       DeliveryConfig
	overrides map[string]ConfigPatch
	held      []*job
	lanes     map[string]*lane // per-app first-attempt FIFOs
	sent      *eventStore
	verifyErr map[string]string
	warned    map[string]bool
	teamID    string
}

type queued struct {
	ev  service.Event
	gen int64
}

// New starts an Engine. Close it to stop its workers.
func New(o Options) *Engine {
	if o.Clock == nil {
		o.Clock = SystemClock{}
	}
	if o.Transport == nil {
		o.Transport = NewHTTPTransport(o.Clock)
	}
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.Config.TimeoutMS == 0 {
		o.Config = DefaultDeliveryConfig(3*time.Second, nil)
	}
	if o.TraceSize < 1 {
		o.TraceSize = defaultTraceSize
	}
	if o.QueueSize < 1 {
		o.QueueSize = defaultQueueSize
	}
	root, stop := context.WithCancel(context.Background())
	e := &Engine{
		src: o.Source, tr: o.Transport, sock: o.Socket, clock: o.Clock, policy: o.Policy, log: o.Log,
		defaults: o.Config, trace: NewTrace(o.TraceSize), queue: make(chan queued, o.QueueSize),
		sem: make(chan struct{}, maxInFlight), root: root, rootStop: stop, loopDone: make(chan struct{}),
		cfg: o.Config, overrides: map[string]ConfigPatch{}, lanes: map[string]*lane{}, sent: newEventStore(o.TraceSize),
		verifyErr: map[string]string{}, warned: map[string]bool{},
	}
	e.ctx, e.cancel = context.WithCancel(root)
	go e.loop()
	return e
}

// Publish implements Dispatcher: O(1), never blocks. A full queue drops the
// event and counts it (see Dropped).
func (e *Engine) Publish(ev service.Event) {
	// Read the generation BEFORE the paused check: a Reset (gen bump) always runs
	// while paused, so an event that sees "not paused" carries a generation no
	// newer than that Reset's and is dropped by fanout if the Reset overtook it.
	gen := e.gen.Load()
	if e.paused.Load() {
		return
	}
	select {
	case e.queue <- queued{ev: ev, gen: gen}:
	default:
		n := e.dropped.Add(1)
		e.log.Warn("event queue full, dropping event", slog.Int64("dropped_total", n))
	}
}

// Dropped counts events lost to a full queue.
func (e *Engine) Dropped() int64 { return e.dropped.Load() }

// Pause implements Dispatcher: Publish becomes a no-op until Resume.
func (e *Engine) Pause() { e.paused.Store(true) }

// Resume implements Dispatcher.
func (e *Engine) Resume() { e.paused.Store(false) }

// Reset implements Dispatcher: cancels in-flight attempts and timers, drops
// queued and held events, clears the trace (Seq keeps counting) and restores
// the default delivery config. Results of attempts already in flight are not recorded.
func (e *Engine) Reset() {
	e.mu.Lock()
	e.cancel()
	e.gen.Add(1)
	e.ctx, e.cancel = context.WithCancel(e.root)
	e.held = nil
	e.lanes = map[string]*lane{} // queued first attempts die with their (cancelled) context
	e.sent = newEventStore(e.sent.cap)
	e.cfg = e.defaults
	e.overrides = map[string]ConfigPatch{}
	e.verifyErr = map[string]string{}
	e.warned = map[string]bool{}
	e.mu.Unlock()
	for drained := false; !drained; {
		select {
		case <-e.queue:
		default:
			drained = true
		}
	}
	e.trace.Clear()
}

// Close implements Dispatcher: stops the fan-out loop and cancels deliveries.
func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closed = true
		e.mu.Unlock()
		e.rootStop()
		<-e.loopDone
		e.wg.Wait()
	})
	return nil
}

func (e *Engine) loop() {
	defer close(e.loopDone)
	for {
		select {
		case <-e.root.Done():
			return
		case q := <-e.queue:
			e.fanout(q)
		}
	}
}

func (e *Engine) fanout(q queued) {
	if q.gen != e.gen.Load() {
		return
	}
	team, ok := e.team()
	if !ok {
		return
	}
	for _, r := range Route(q.ev, e.src.Apps(), team, e.clock.Now()) {
		eventID := newEventID()
		body, err := Envelope(r.App, team, eventID, e.clock.Now(), r.Event)
		if err != nil {
			e.log.Error("encode event", slog.Any("error", err))
			continue
		}
		e.enqueue(q.gen, outgoing{app: r.App, eventID: eventID, eventType: r.Type, payload: body})
	}
}

func (e *Engine) team() (string, bool) {
	e.mu.Lock()
	id := e.teamID
	e.mu.Unlock()
	if id != "" {
		return id, true
	}
	t, err := e.src.Team(e.root)
	if err != nil {
		e.log.Error("load team for events", slog.Any("error", err))
		return "", false
	}
	e.mu.Lock()
	e.teamID = t.ID
	e.mu.Unlock()
	return t.ID, true
}

// configLocked is app's effective delivery config.
func (e *Engine) configLocked(appID string) DeliveryConfig {
	o, ok := e.overrides[appID]
	if !ok {
		return e.cfg
	}
	eff, err := e.cfg.Apply(o)
	if err != nil { // validated on write; fall back rather than fail a delivery
		return e.cfg
	}
	return eff
}

// Resolve reports where app's events go (zero Target for Socket Mode apps).
func (e *Engine) Resolve(app service.App) Target {
	if app.SocketMode {
		return Target{}
	}
	return e.policy.Resolve(app.RequestURL, app.DeliverTo)
}

// Deliveries lists the trace; see Trace.List.
func (e *Engine) Deliveries(f Filter) (items []Delivery, next, generation int64) {
	items, next = e.trace.List(f)
	return items, next, e.trace.Generation()
}

// WaitDeliveries is Deliveries that long-polls; see Trace.Wait.
func (e *Engine) WaitDeliveries(ctx context.Context, f Filter, minCount int, timeout time.Duration) (items []Delivery, next, generation int64) {
	return e.trace.Wait(ctx, f, minCount, timeout)
}

// Delivery returns one delivery by id.
func (e *Engine) Delivery(id string) (Delivery, bool) { return e.trace.Get(id) }

// ClearDeliveries empties the trace (Seq keeps counting) and wakes long-polls.
func (e *Engine) ClearDeliveries() { e.trace.Clear() }
