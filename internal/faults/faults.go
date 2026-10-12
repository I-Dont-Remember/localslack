// Package faults defines the fault-injection seam for API calls. Leaf
// package: no service or store imports.
package faults

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Request is what an Interceptor may match on: parsed, pre-auth-enforcement.
type Request struct {
	Method     string // Slack method, e.g. chat.postMessage
	HTTPMethod string
	Params     url.Values
	Raw        map[string]any // decoded JSON body, nil otherwise
	UserID     string         // "" when the token did not resolve
	AppID      string
	TokenKind  string // bot|user|app|none|invalid
}

// Override is a fired fault. With Respond false only Delay applies and the
// real method runs afterwards.
type Override struct {
	FaultID string
	Delay   time.Duration
	Respond bool
	Status  int
	Headers http.Header
	Body    map[string]any
}

// Interceptor decides per call whether a fault fires.
type Interceptor interface {
	Intercept(ctx context.Context, req Request) (Override, bool)
}

// Noop never fires.
type Noop struct{}

// Intercept implements Interceptor.
func (Noop) Intercept(context.Context, Request) (Override, bool) { return Override{}, false }

// Set holds fault rules and implements Interceptor. Skeleton: the faults stream
// adds rule storage and matching; until then no fault ever fires.
type Set struct{}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{} }

// Intercept implements Interceptor: no rules, no override.
func (*Set) Intercept(context.Context, Request) (Override, bool) { return Override{}, false }

// Clear drops all rules (reset hook; nothing to drop yet).
func (*Set) Clear() {}
