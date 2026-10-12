// Package control orchestrates cross-cutting operations (reset, snapshot) that
// span the service and the event dispatcher. admin and the UI both call it, so
// they behave identically. It must not import store, api or web.
package control

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"localslack/internal/events"
	"localslack/internal/service"
)

// ErrResetInProgress is returned when a reset is requested while another runs.
var ErrResetInProgress = errors.New("control: reset already in progress")

// Control runs orchestrated operations.
type Control struct {
	svc  *service.Service
	disp events.Dispatcher

	defaultSeed string
	onReset     []func()
	now         func() time.Time

	resetMu sync.Mutex
	gen     atomic.Int64
}

// Option configures a Control.
type Option func(*Control)

// WithSeed sets the seed a reset uses when the request names none (the
// server's configured seed, so a bare reset returns to the startup data).
func WithSeed(seed string) Option { return func(c *Control) { c.defaultSeed = seed } }

// WithOnReset registers hooks run after a successful wipe, in order, to clear
// state held outside the database (call log, faults, snapshots, ...).
func WithOnReset(fns ...func()) Option {
	return func(c *Control) { c.onReset = append(c.onReset, fns...) }
}

// New returns a Control over svc and disp.
func New(svc *service.Service, disp events.Dispatcher, opts ...Option) *Control {
	c := &Control{svc: svc, disp: disp, now: time.Now}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ResetOpts selects what a reset repopulates; extended by the scenario stream.
type ResetOpts struct {
	// All also removes apps (blank server); default keeps them.
	All  bool
	Seed string
}

// ResetResult is the reply of a completed reset (docs/TESTING_API.md, Reset).
type ResetResult struct {
	Mode       string
	Generation int64
	Seed       string
	Counts     service.ResetCounts
	TookMS     int64
}

// Reset wipes and repopulates state with the dispatcher quiesced:
// Pause -> service reset (one tx) -> dispatcher Reset (cancel in-flight, clear
// trace) -> clear externally held state -> Resume. The dispatcher is wiped only
// after the data commits, so a failed reset leaves the trace and in-flight
// deliveries alone (it just resumes). A concurrent Reset fails with
// ErrResetInProgress. No events are emitted.
//
// Known limit: a write committed between the service wipe and Resume is new
// data, but its event is dropped (Publish is paused), so apps never hear of it.
// Callers resetting under load should expect that.
func (c *Control) Reset(ctx context.Context, o ResetOpts) (ResetResult, error) {
	if !c.resetMu.TryLock() {
		return ResetResult{}, ErrResetInProgress
	}
	defer c.resetMu.Unlock()
	start := c.now()

	c.disp.Pause()
	defer c.disp.Resume()

	seed := o.Seed
	if seed == "" {
		seed = c.defaultSeed
	}
	res, err := c.svc.Reset(ctx, service.ResetOpts{All: o.All, Seed: seed})
	if err != nil {
		return ResetResult{}, err
	}
	c.disp.Reset()
	for _, fn := range c.onReset {
		fn()
	}
	mode := "partial"
	if o.All {
		mode = "all"
	}
	return ResetResult{
		Mode: mode, Generation: c.gen.Add(1), Seed: res.Seed, Counts: res.Counts,
		TookMS: c.now().Sub(start).Milliseconds(),
	}, nil
}

// Snapshot returns every conversation with members and messages. Shell: the
// snapshot stream swaps this for the Fixture export.
func (c *Control) Snapshot(ctx context.Context) ([]service.ConversationState, error) {
	return c.svc.Snapshot(ctx)
}
