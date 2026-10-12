package events_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
)

func TestDefaultDeliveryConfig(t *testing.T) {
	t.Parallel()

	got := events.DefaultDeliveryConfig(3*time.Second, []time.Duration{0, time.Minute, 5 * time.Minute})
	want := events.DeliveryConfig{
		Signature: "valid", StaleSeconds: 600, TimeoutMS: 3000, RetryDelaysMS: []int64{0, 60000, 300000},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("default config (-want +got):\n%s", diff)
	}
	b, err := json.Marshal(events.DefaultDeliveryConfig(time.Second, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"signature":"valid","stale_seconds":600,"timeout_ms":1000,"retry_delays_ms":[],"paused":false}`; string(b) != want {
		t.Errorf("JSON = %s, want %s", b, want)
	}
}

func TestDeliveryConfigPatch(t *testing.T) {
	t.Parallel()

	base := events.DefaultDeliveryConfig(3*time.Second, []time.Duration{0})
	tests := []struct {
		name    string
		patch   string
		want    events.DeliveryConfig
		wantErr string // field
	}{
		{"empty patch keeps all", `{}`, base, ""},
		{"signature", `{"signature":"stale"}`, withSig(base, "stale"), ""},
		{"paused", `{"paused":true}`, withPaused(base), ""},
		{"timeout and delays", `{"timeout_ms":50,"retry_delays_ms":[1,2]}`, events.DeliveryConfig{
			Signature: "valid", StaleSeconds: 600, TimeoutMS: 50, RetryDelaysMS: []int64{1, 2},
		}, ""},
		{"empty delays disables retries", `{"retry_delays_ms":[]}`, events.DeliveryConfig{
			Signature: "valid", StaleSeconds: 600, TimeoutMS: 3000, RetryDelaysMS: []int64{},
		}, ""},
		{"bad signature", `{"signature":"nope"}`, base, "signature"},
		{"zero timeout", `{"timeout_ms":0}`, base, "timeout_ms"},
		{"negative delay", `{"retry_delays_ms":[-1]}`, base, "retry_delays_ms"},
		{"negative stale", `{"stale_seconds":-5}`, base, "stale_seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var p events.ConfigPatch
			if err := json.Unmarshal([]byte(tt.patch), &p); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			got, err := base.Apply(p)
			if tt.wantErr != "" {
				var ce *events.ConfigError
				if !errors.As(err, &ce) || ce.Field != tt.wantErr {
					t.Fatalf("Apply err = %v, want ConfigError on %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("config (-want +got):\n%s", diff)
			}
		})
	}
}

func withSig(c events.DeliveryConfig, s string) events.DeliveryConfig {
	c.Signature = s
	return c
}

func withPaused(c events.DeliveryConfig) events.DeliveryConfig {
	c.Paused = true
	return c
}

func TestDeliveryConfigDurations(t *testing.T) {
	t.Parallel()

	c := events.DeliveryConfig{TimeoutMS: 1500, RetryDelaysMS: []int64{0, 250}}
	if got := c.Timeout(); got != 1500*time.Millisecond {
		t.Errorf("Timeout = %v", got)
	}
	if diff := cmp.Diff([]time.Duration{0, 250 * time.Millisecond}, c.RetryDelays()); diff != "" {
		t.Errorf("RetryDelays (-want +got):\n%s", diff)
	}
}
