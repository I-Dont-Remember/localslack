package faults

import (
	"fmt"
	"strings"
)

// MaxDelayMS caps Action.DelayMS.
const MaxDelayMS = 60_000

// Spec is a rule as clients write it.
//
//	{"method":"chat.postMessage","nth":3,"action":{"status":429,"retry_after":2}}
type Spec struct {
	Method string `json:"method"` // Slack method or "*"
	Match  Match  `json:"match,omitzero"`
	Nth    int    `json:"nth,omitempty"`   // first eligible matching call (1-based)
	Every  int    `json:"every,omitempty"` // then every Nth matching call
	Times  int    `json:"times,omitempty"` // max firings
	Once   bool   `json:"once,omitempty"`  // same as times:1
	Action Action `json:"action"`
}

// Match narrows a rule; empty fields match anything.
type Match struct {
	Channel   string `json:"channel,omitempty"`
	AppID     string `json:"app_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	TokenKind string `json:"token_kind,omitempty"`
}

// Action is what a firing rule does. Status 0 with no error/body means
// delay only (the real method then runs).
type Action struct {
	Status     int            `json:"status,omitempty"`
	Error      string         `json:"error,omitempty"`
	RetryAfter int            `json:"retry_after,omitempty"` // seconds, Retry-After header
	DelayMS    int            `json:"delay_ms,omitempty"`
	Body       map[string]any `json:"body,omitempty"`
}

// Rule is a stored Spec with its counters.
type Rule struct {
	ID string `json:"id"`
	Spec
	Seen  int  `json:"seen"`  // matching calls while live
	Fired int  `json:"fired"` // times it won
	Spent bool `json:"spent"` // hit its firing limit; no longer matches
}

// Detail locates one validation problem (JSON pointer into the request body).
type Detail struct {
	Message string `json:"message"`
	Pointer string `json:"pointer"`
}

// ValidationError lists everything wrong with a Spec.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Details))
	for _, d := range e.Details {
		parts = append(parts, d.Pointer+": "+d.Message)
	}
	return "invalid fault: " + strings.Join(parts, "; ")
}

func (sp Spec) validate() error {
	var ds []Detail
	bad := func(ptr, format string, args ...any) {
		ds = append(ds, Detail{Pointer: ptr, Message: fmt.Sprintf(format, args...)})
	}
	if sp.Method == "" {
		bad("/method", "required: a Slack method name or \"*\"")
	}
	for ptr, n := range map[string]int{"/nth": sp.Nth, "/every": sp.Every, "/times": sp.Times} {
		if n < 0 {
			bad(ptr, "must be >= 0")
		}
	}
	if sp.Once && sp.Times > 1 {
		bad("/once", "conflicts with times=%d (once means times=1)", sp.Times)
	}
	ds = append(ds, sp.Action.validate()...)
	if len(ds) > 0 {
		return &ValidationError{Details: ds}
	}
	return nil
}

func (a Action) validate() []Detail {
	var ds []Detail
	bad := func(ptr, msg string) { ds = append(ds, Detail{Pointer: ptr, Message: msg}) }
	if a.Status != 0 && a.Status != 200 && (a.Status < 400 || a.Status > 599) {
		bad("/action/status", "must be 200 or 400-599")
	}
	if a.DelayMS < 0 || a.DelayMS > MaxDelayMS {
		bad("/action/delay_ms", fmt.Sprintf("must be 0-%d", MaxDelayMS))
	}
	if a.RetryAfter < 0 {
		bad("/action/retry_after", "must be >= 0")
	}
	respond := a.Error != "" || a.Body != nil || a.Status >= 400
	if a.Status == 200 && !respond || a.Status == 0 && !respond && a.DelayMS == 0 {
		bad("/action", "needs an error, body, 4xx/5xx status, or delay_ms (status 200 alone does nothing)")
	}
	return ds
}
