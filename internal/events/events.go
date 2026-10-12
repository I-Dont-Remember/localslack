// Package events delivers service events to apps. This is the skeleton: the
// Dispatcher and Transport seams plus a no-op Dispatcher. The events and
// socket streams add the queue, workers, signing and transports behind them.
// It must not import api, web or store.
package events

import (
	"context"
	"encoding/json"
	"time"

	"localslack/internal/service"
)

// Dispatcher turns service events into deliveries.
type Dispatcher interface {
	// Publish accepts an event. It must be O(1) and never block: it is called
	// inline from the write path (see service.Service.Subscribe).
	Publish(service.Event)
	// Pause stops accepting and sending events (Publish becomes a no-op until
	// Resume), as control.Reset does around a wipe.
	Pause()
	// Reset cancels in-flight deliveries and timers and bumps the generation so
	// late results are not recorded.
	Reset()
	// Resume undoes Pause.
	Resume()
	// Close stops workers.
	Close() error
}

// Transport performs one delivery attempt (HTTP POST, socket frame, ...).
type Transport interface {
	Deliver(ctx context.Context, d Delivery) Attempt
}

// Delivery states.
const (
	StatePending   = "pending"
	StateRetrying  = "retrying"
	StateDelivered = "delivered"
	StateFailed    = "failed"
)

// Delivery is one event addressed to one app, with its attempt history
// (docs/TESTING_API.md, Events).
type Delivery struct {
	ID        string          `json:"id"`
	Seq       int64           `json:"seq"`
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	AppID     string          `json:"app_id"`
	Transport string          `json:"transport"` // http|socket
	State     string          `json:"state"`
	Injected  bool            `json:"injected"`
	Payload   json.RawMessage `json:"payload"`
	Attempts  []Attempt       `json:"attempts"`

	// Spec is how to send it; internal, never serialized.
	Spec Spec `json:"-"`
}

// Spec is the recipe for one attempt, filled by the dispatcher for the Transport.
type Spec struct {
	URL, Secret string
	// Mode is a signature mode (SigValid ...); StaleSeconds backdates SigStale.
	Mode         string
	StaleSeconds int
	// RetryNum and RetryReason become X-Slack-Retry-Num/-Reason when RetryNum > 0.
	RetryNum    int
	RetryReason string
}

// Attempt is one try at delivering.
type Attempt struct {
	N             int       `json:"n"`
	At            time.Time `json:"at"`
	Status        int       `json:"status"`
	LatencyMS     int64     `json:"latency_ms"`
	Error         string    `json:"error"`
	RetryReason   string    `json:"retry_reason"`
	SignatureMode string    `json:"signature_mode"`
	Timestamp     string    `json:"timestamp"`
	Signature     string    `json:"signature"`
	ResponseBody  string    `json:"response_body"`

	// NoRetry is set when the receiver answered X-Slack-No-Retry: 1.
	NoRetry bool `json:"-"`
	// FailReason classifies a failure for the next attempt's X-Slack-Retry-Reason.
	FailReason string `json:"-"`
}

// OK reports whether the attempt was acknowledged (a 2xx response, no error).
func (a Attempt) OK() bool { return a.Error == "" && a.Status >= 200 && a.Status < 300 }

// Noop drops every event.
type Noop struct{}

// Publish implements Dispatcher.
func (Noop) Publish(service.Event) {}

// Pause implements Dispatcher.
func (Noop) Pause() {}

// Reset implements Dispatcher.
func (Noop) Reset() {}

// Resume implements Dispatcher.
func (Noop) Resume() {}

// Close implements Dispatcher.
func (Noop) Close() error { return nil }
