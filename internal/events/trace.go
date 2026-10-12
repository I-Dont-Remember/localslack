package events

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Trace list limits (docs/TESTING_API.md, Conventions).
const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

// Filter selects deliveries; zero fields match everything.
type Filter struct {
	AppID, EventType, State string
	// Since is exclusive: only deliveries with Seq > Since.
	Since int64
	Limit int
}

func (f Filter) match(d *Delivery) bool {
	return d.Seq > f.Since &&
		(f.AppID == "" || d.AppID == f.AppID) &&
		(f.EventType == "" || d.EventType == f.EventType) &&
		(f.State == "" || d.State == f.State)
}

// Trace is a bounded, concurrency-safe record of deliveries with long-poll
// waits. Seq is monotonic across Clear; Generation changes on Clear.
type Trace struct {
	mu      sync.Mutex
	cap     int
	items   []*Delivery // oldest first
	byID    map[string]*Delivery
	seq     int64
	gen     int64
	changed chan struct{} // closed and replaced on every mutation
}

// NewTrace returns a Trace holding at most size deliveries.
func NewTrace(size int) *Trace {
	if size < 1 {
		size = 1
	}
	return &Trace{cap: size, byID: map[string]*Delivery{}, changed: make(chan struct{})}
}

func (t *Trace) notify() {
	close(t.changed)
	t.changed = make(chan struct{})
}

// Add records d, assigning its Seq and ID, and returns the stored copy.
func (t *Trace) Add(d Delivery) Delivery {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	d.Seq, d.ID = t.seq, "dl_"+strconv.FormatInt(t.seq, 10)
	if d.Attempts == nil {
		d.Attempts = []Attempt{}
	}
	stored := d
	t.items = append(t.items, &stored)
	t.byID[d.ID] = &stored
	if len(t.items) > t.cap {
		delete(t.byID, t.items[0].ID)
		t.items = t.items[1:]
	}
	t.notify()
	return clone(&stored)
}

// Update mutates the delivery with id under the lock; unknown ids are ignored.
func (t *Trace) Update(id string, fn func(*Delivery)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if d, ok := t.byID[id]; ok {
		fn(d)
		t.notify()
	}
}

// Get returns a copy of the delivery with id.
func (t *Trace) Get(id string) (Delivery, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	d, ok := t.byID[id]
	if !ok {
		return Delivery{}, false
	}
	return clone(d), true
}

// Clear drops every delivery and bumps the generation; Seq keeps counting.
func (t *Trace) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items, t.byID = nil, map[string]*Delivery{}
	t.gen++
	t.notify()
}

// Generation is bumped by Clear.
func (t *Trace) Generation() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.gen
}

// List returns copies of matching deliveries, oldest first, and next_since: the
// last returned Seq, or the input Since when nothing matched.
func (t *Trace) List(f Filter) ([]Delivery, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.listLocked(f)
}

func (t *Trace) listLocked(f Filter) ([]Delivery, int64) {
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	out := []Delivery{}
	for _, d := range t.items {
		if len(out) == limit {
			break
		}
		if f.match(d) {
			out = append(out, clone(d))
		}
	}
	next := f.Since
	if n := len(out); n > 0 {
		next = out[n-1].Seq
	}
	return out, next
}

// Wait is List that parks until at least minCount deliveries match, the
// timeout passes, ctx ends, or the trace is cleared (the new generation is
// returned). It returns whatever matches when it stops.
func (t *Trace) Wait(ctx context.Context, f Filter, minCount int, timeout time.Duration) (items []Delivery, next, generation int64) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	t.mu.Lock()
	startGen := t.gen
	for {
		items, next = t.listLocked(f)
		if len(items) >= minCount || t.gen != startGen {
			break
		}
		wake := t.changed
		t.mu.Unlock()
		select {
		case <-wake:
		case <-deadline.C:
			t.mu.Lock()
			items, next = t.listLocked(f)
			generation = t.gen
			t.mu.Unlock()
			return items, next, generation
		case <-ctx.Done():
			t.mu.Lock()
			items, next = t.listLocked(f)
			generation = t.gen
			t.mu.Unlock()
			return items, next, generation
		}
		t.mu.Lock()
	}
	generation = t.gen
	t.mu.Unlock()
	return items, next, generation
}

func clone(d *Delivery) Delivery {
	c := *d
	c.Attempts = slices.Clone(d.Attempts)
	return c
}
