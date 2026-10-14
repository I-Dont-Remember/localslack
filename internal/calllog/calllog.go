// Package calllog defines the API call record and the Recorder seam. Leaf
// package: no service or store imports.
package calllog

import "time"

// Actor is who made a call, as far as the token revealed.
type Actor struct {
	UserID    string `json:"user_id"`
	AppID     string `json:"app_id"`
	TokenKind string `json:"token_kind"` // bot|user|app|none
}

// CallRecord is one Web API call, see docs/TESTING_API.md (call log).
// Recorders assign Seq and must redact secrets from Params.
type CallRecord struct {
	Seq        int64          `json:"seq"`
	Time       time.Time      `json:"time"`
	RequestID  string         `json:"request_id"`
	Method     string         `json:"method"`
	HTTPMethod string         `json:"http_method"`
	Status     int            `json:"status"`
	OK         bool           `json:"ok"`
	Error      string         `json:"error"`
	Params     map[string]any `json:"params"`
	Actor      Actor          `json:"actor"`
	Response   map[string]any `json:"response"` // nil when over MaxResponseBytes
	// ResponseTruncated is set when the body exceeded MaxResponseBytes and was dropped.
	ResponseTruncated bool   `json:"response_truncated"`
	DurationMS        int64  `json:"duration_ms"`
	FaultID           string `json:"fault_id"`
	NotImplemented    bool   `json:"not_implemented"`
}

// MaxResponseBytes caps the response body a record may carry.
const MaxResponseBytes = 64 << 10

// Recorder receives every completed API call. Must be fast and non-blocking.
type Recorder interface {
	Record(CallRecord)
}

// Noop discards records.
type Noop struct{}

// Record implements Recorder.
func (Noop) Record(CallRecord) {}
