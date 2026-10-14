package events

import (
	"fmt"
	"slices"
	"time"
)

// DeliveryConfig is the delivery-config document (docs/TESTING_API.md): how
// events are signed, timed out and retried, and whether they are held.
type DeliveryConfig struct {
	Signature     string  `json:"signature"`
	StaleSeconds  int     `json:"stale_seconds"`
	TimeoutMS     int64   `json:"timeout_ms"`
	RetryDelaysMS []int64 `json:"retry_delays_ms"`
	Paused        bool    `json:"paused"`
}

// DefaultDeliveryConfig builds the config for the process-level settings.
func DefaultDeliveryConfig(timeout time.Duration, delays []time.Duration) DeliveryConfig {
	c := DeliveryConfig{Signature: SigValid, StaleSeconds: 600, TimeoutMS: timeout.Milliseconds(), RetryDelaysMS: []int64{}}
	for _, d := range delays {
		c.RetryDelaysMS = append(c.RetryDelaysMS, d.Milliseconds())
	}
	return c
}

// Timeout is the per-attempt deadline.
func (c DeliveryConfig) Timeout() time.Duration { return time.Duration(c.TimeoutMS) * time.Millisecond }

// RetryDelays are the waits before each retry.
func (c DeliveryConfig) RetryDelays() []time.Duration {
	out := make([]time.Duration, 0, len(c.RetryDelaysMS))
	for _, ms := range c.RetryDelaysMS {
		out = append(out, time.Duration(ms)*time.Millisecond)
	}
	return out
}

// ConfigPatch is a partial DeliveryConfig: nil fields keep their value. As a
// per-app override it is sparse: nil fields inherit the global config live.
type ConfigPatch struct {
	Signature     *string  `json:"signature,omitempty"`
	StaleSeconds  *int     `json:"stale_seconds,omitempty"`
	TimeoutMS     *int64   `json:"timeout_ms,omitempty"`
	RetryDelaysMS *[]int64 `json:"retry_delays_ms,omitempty"`
	Paused        *bool    `json:"paused,omitempty"`
}

// IsZero reports whether no field is set.
func (p ConfigPatch) IsZero() bool {
	return p.Signature == nil && p.StaleSeconds == nil && p.TimeoutMS == nil && p.RetryDelaysMS == nil && p.Paused == nil
}

// Merge returns p with q's set fields on top (sparse-override accumulation).
func (p ConfigPatch) Merge(q ConfigPatch) ConfigPatch {
	if q.Signature != nil {
		p.Signature = q.Signature
	}
	if q.StaleSeconds != nil {
		p.StaleSeconds = q.StaleSeconds
	}
	if q.TimeoutMS != nil {
		p.TimeoutMS = q.TimeoutMS
	}
	if q.RetryDelaysMS != nil {
		p.RetryDelaysMS = q.RetryDelaysMS
	}
	if q.Paused != nil {
		p.Paused = q.Paused
	}
	return p
}

// ConfigError is an invalid delivery-config field.
type ConfigError struct{ Field, Message string }

// Error implements error.
func (e *ConfigError) Error() string { return e.Field + ": " + e.Message }

// Apply returns c with p merged in, or a *ConfigError.
func (c DeliveryConfig) Apply(p ConfigPatch) (DeliveryConfig, error) {
	out := c
	out.RetryDelaysMS = slices.Clone(c.RetryDelaysMS)
	if p.Signature != nil {
		if !slices.Contains([]string{SigValid, SigInvalid, SigStale, SigMissing}, *p.Signature) {
			return c, &ConfigError{"signature", fmt.Sprintf("must be one of valid, invalid, stale, missing (got %q)", *p.Signature)}
		}
		out.Signature = *p.Signature
	}
	if p.StaleSeconds != nil {
		if *p.StaleSeconds < 0 {
			return c, &ConfigError{"stale_seconds", "must not be negative"}
		}
		out.StaleSeconds = *p.StaleSeconds
	}
	if p.TimeoutMS != nil {
		if *p.TimeoutMS < 1 {
			return c, &ConfigError{"timeout_ms", "must be positive"}
		}
		out.TimeoutMS = *p.TimeoutMS
	}
	if p.RetryDelaysMS != nil {
		if slices.ContainsFunc(*p.RetryDelaysMS, func(ms int64) bool { return ms < 0 }) {
			return c, &ConfigError{"retry_delays_ms", "must not contain negative delays"}
		}
		out.RetryDelaysMS = slices.Clone(*p.RetryDelaysMS)
	}
	if p.Paused != nil {
		out.Paused = *p.Paused
	}
	if out.RetryDelaysMS == nil {
		out.RetryDelaysMS = []int64{}
	}
	return out, nil
}
