package calllog

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	defaultLimit = 100
	maxLimit     = 1000
	maxTimeout   = 30 * time.Second
	redacted     = "[redacted]"
)

// Query filters the log; zero values mean "no filter". Since is an exclusive seq.
// With Min > 0 the call parks (long poll) until Min records match, Timeout
// elapses (capped at 30s), the log is cleared, or ctx ends.
type Query struct {
	Method, Channel, AppID, Error string
	OK                            *bool
	Since                         int64
	Limit                         int
	Min                           int
	Timeout                       time.Duration
}

// Result is a Query answer, oldest first.
type Result struct {
	Calls []CallRecord
	// NextSince is the last returned seq (or the input Since when empty).
	NextSince  int64
	Generation int64
	// Truncated is set when more matches existed than Limit allowed.
	Truncated bool
}

// Log is a bounded in-memory Recorder. Seq is monotonic across Clear.
type Log struct {
	mu      sync.Mutex
	buf     []CallRecord // ring: oldest at start
	start   int
	size    int
	seq     int64
	gen     int64
	wake    chan struct{} // closed and replaced on every Record/Clear
	waiters int
}

// New returns a Log holding at most capacity records (minimum 1).
func New(capacity int) *Log {
	return &Log{buf: make([]CallRecord, max(capacity, 1)), wake: make(chan struct{})}
}

// Record stores r with the next seq, redacting secrets (r itself is not modified).
func (l *Log) Record(r CallRecord) {
	r.Params = redactMap(r.Params)
	r.Response = redactMap(r.Response)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	r.Seq = l.seq
	if l.size < len(l.buf) {
		l.buf[(l.start+l.size)%len(l.buf)] = r
		l.size++
	} else {
		l.buf[l.start] = r
		l.start = (l.start + 1) % len(l.buf)
	}
	l.broadcast()
}

// Clear drops all records. Seq keeps counting and Generation advances so
// clients can tell; parked pollers wake.
func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.start, l.size = 0, 0
	clear(l.buf)
	l.gen++
	l.broadcast()
}

// Waiters reports how many Query calls are parked (for tests).
func (l *Log) Waiters() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.waiters
}

func (l *Log) broadcast() {
	close(l.wake)
	l.wake = make(chan struct{})
}

// Query returns matching records, long-polling when q.Min > 0.
func (l *Log) Query(ctx context.Context, q Query) Result {
	limit := clampLimit(q.Limit)
	res, wake, gen0 := l.snapshot(q, limit)
	if q.Min <= 0 || len(res.Calls) >= q.Min {
		return res
	}
	timer := time.NewTimer(min(q.Timeout, maxTimeout))
	defer timer.Stop()
	for len(res.Calls) < q.Min {
		if !l.wait(ctx, wake, timer.C) {
			return res
		}
		var gen int64
		res, wake, gen = l.snapshot(q, limit)
		if gen != gen0 { // cleared while parked: let the client see the new generation
			return res
		}
	}
	return res
}

// wait parks until wake closes (true) or the timer/ctx ends (false).
func (l *Log) wait(ctx context.Context, wake <-chan struct{}, timeout <-chan time.Time) bool {
	l.mu.Lock()
	l.waiters++
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.waiters--
		l.mu.Unlock()
	}()
	select {
	case <-wake:
		return true
	case <-timeout:
		return false
	case <-ctx.Done():
		return false
	}
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	}
	return n
}

// snapshot filters under the lock; it returns the wake channel to wait on and the generation seen.
func (l *Log) snapshot(q Query, limit int) (Result, <-chan struct{}, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	res := Result{NextSince: q.Since, Generation: l.gen}
	for i := range l.size {
		r := l.buf[(l.start+i)%len(l.buf)]
		if r.Seq <= q.Since || !q.matches(r) {
			continue
		}
		if len(res.Calls) == limit {
			res.Truncated = true
			break
		}
		res.Calls = append(res.Calls, r)
		res.NextSince = r.Seq
	}
	return res, l.wake, l.gen
}

func (q Query) matches(r CallRecord) bool {
	ch, _ := r.Params["channel"].(string)
	switch {
	case q.Method != "" && r.Method != q.Method,
		q.Channel != "" && ch != q.Channel,
		q.AppID != "" && r.Actor.AppID != q.AppID,
		q.Error != "" && r.Error != q.Error,
		q.OK != nil && r.OK != *q.OK:
		return false
	}
	return true
}

func isSecretKey(k string) bool {
	switch strings.ToLower(k) {
	case "token", "client_secret", "signing_secret", "authorization", "cookie":
		return true
	}
	return false
}

// redactMap deep-copies m with secret-named values replaced.
func redactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSecretKey(k) {
			out[k] = redacted
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

func redactValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return redactMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactValue(e)
		}
		return out
	}
	return v
}
