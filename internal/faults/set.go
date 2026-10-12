package faults

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Set holds fault rules and implements Interceptor. Safe for concurrent use.
type Set struct {
	mu    sync.Mutex
	rules []*rule
	next  int // last issued id number; never reset so ids are not reused
}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{} }

// Add validates and stores a rule, returning its snapshot.
func (s *Set) Add(spec Spec) (Rule, error) {
	if err := spec.validate(); err != nil {
		return Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	r := &rule{id: "f_" + strconv.Itoa(s.next), spec: spec}
	s.rules = append(s.rules, r)
	return r.snapshot(), nil
}

// List returns every rule (spent ones included) in creation order.
func (s *Set) List() []Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r.snapshot())
	}
	return out
}

// Delete removes one rule and reports whether it existed.
func (s *Set) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.id == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			return true
		}
	}
	return false
}

// Clear drops all rules (reset hook).
func (s *Set) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = nil
}

// Intercept implements Interceptor. Every live rule matching the request
// counts it; the first (creation order) whose counters say "fire" wins.
func (s *Set) Intercept(_ context.Context, req Request) (Override, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var winner *rule
	for _, r := range s.rules {
		if r.spent() || !r.spec.matches(req) {
			continue
		}
		r.seen++
		if winner == nil && r.spec.due(r.seen) {
			r.fired++
			winner = r
		}
	}
	if winner == nil {
		return Override{}, false
	}
	return winner.spec.Action.override(winner.id), true
}

type rule struct {
	id          string
	spec        Spec
	seen, fired int
}

func (r *rule) spent() bool {
	limit := r.spec.limit()
	return limit > 0 && r.fired >= limit
}

func (r *rule) snapshot() Rule {
	return Rule{ID: r.id, Spec: r.spec, Seen: r.seen, Fired: r.fired, Spent: r.spent()}
}

// matches checks method and match fields.
func (sp Spec) matches(req Request) bool {
	if sp.Method != "*" && sp.Method != req.Method {
		return false
	}
	m := sp.Match
	return (m.Channel == "" || m.Channel == req.Params.Get("channel")) &&
		(m.AppID == "" || m.AppID == req.AppID) &&
		(m.UserID == "" || m.UserID == req.UserID) &&
		(m.TokenKind == "" || m.TokenKind == req.TokenKind)
}

// limit is the max number of firings; 0 means unlimited.
func (sp Spec) limit() int {
	switch {
	case sp.Once:
		return 1
	case sp.Times > 0:
		return sp.Times
	case sp.Nth > 0 && sp.Every == 0:
		return 1 // "the nth call"
	}
	return 0
}

// due says whether the seen-th matching call fires: from the first eligible
// call (nth, else every, else 1) onward, every `every` calls (default each).
func (sp Spec) due(seen int) bool {
	start := max(sp.Nth, sp.Every, 1)
	if sp.Nth > 0 {
		start = sp.Nth
	}
	if seen < start {
		return false
	}
	return sp.Every <= 1 || (seen-start)%sp.Every == 0
}

func (a Action) override(id string) Override {
	ov := Override{FaultID: id, Delay: time.Duration(a.DelayMS) * time.Millisecond}
	if a.Status == 0 && a.Error == "" && a.Body == nil {
		return ov // delay only: the real method runs afterwards
	}
	ov.Respond = true
	ov.Status = a.Status
	if ov.Status == 0 {
		ov.Status = http.StatusOK
	}
	ov.Body = a.Body
	if ov.Body == nil {
		ov.Body = map[string]any{"ok": false, "error": a.errorCode()}
	}
	switch {
	case a.RetryAfter > 0:
		ov.Headers = http.Header{"Retry-After": {strconv.Itoa(a.RetryAfter)}}
	case ov.Status == http.StatusTooManyRequests:
		ov.Headers = http.Header{"Retry-After": {"1"}}
	}
	return ov
}

func (a Action) errorCode() string {
	switch {
	case a.Error != "":
		return a.Error
	case a.Status == http.StatusTooManyRequests:
		return "ratelimited"
	case a.Status >= http.StatusInternalServerError:
		return "internal_error"
	}
	return "fault_injected"
}
