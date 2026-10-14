package cli

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/config"
)

func TestStartCmdApply(t *testing.T) {
	t.Parallel()

	base := config.Config{
		Addr: "127.0.0.1:3232", EventTimeout: 3 * time.Second, EventRetryDelays: []time.Duration{0, time.Minute},
		CallLogSize: 10000, DeliveryLogSize: 2000,
	}
	tests := []struct {
		name    string
		cmd     StartCmd
		want    config.Config
		wantErr bool
	}{
		{"no flags keeps config", StartCmd{}, base, false},
		{
			"all flags override",
			StartCmd{
				Addr: "0.0.0.0:1", AppManifest: []string{"a.json"}, EventTimeout: time.Second, EventRetryDelays: "0s,100ms",
				Scenario: "s.json", CallLogSize: 7, DeliveryLogSize: 8, StrictTokens: true,
				DeliverTo: "http://localhost:3000", AllowRemoteRequestURLs: true,
			},
			config.Config{
				Addr: "0.0.0.0:1", AppManifest: []string{"a.json"}, EventTimeout: time.Second,
				EventRetryDelays: []time.Duration{0, 100 * time.Millisecond}, Scenario: "s.json",
				CallLogSize: 7, DeliveryLogSize: 8, StrictTokens: true,
				DeliverTo: "http://localhost:3000", AllowRemoteRequestURLs: true,
			},
			false,
		},
		{"bad delays", StartCmd{EventRetryDelays: "x"}, config.Config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := base
			err := tt.cmd.apply(&got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("apply err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("apply (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestStartCmdDetachedArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  StartCmd
		want []string
	}{
		{"none", StartCmd{Detach: true}, []string{"start"}},
		{
			"everything passes through",
			StartCmd{
				Detach: true, Addr: "a:1", AppManifest: []string{"x.json", "y.yaml"}, EventTimeout: 500 * time.Millisecond,
				EventRetryDelays: "0s,1s", Scenario: "s.json", CallLogSize: 5, DeliveryLogSize: 6, StrictTokens: true,
				DeliverTo: "http://localhost:3000", AllowRemoteRequestURLs: true,
			},
			[]string{
				"start", "--addr", "a:1", "--app-manifest", "x.json", "--app-manifest", "y.yaml",
				"--event-timeout", "500ms", "--event-retry-delays", "0s,1s", "--scenario", "s.json",
				"--call-log-size", "5", "--delivery-log-size", "6", "--strict-tokens",
				"--deliver-to", "http://localhost:3000", "--allow-remote-request-urls",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, tt.cmd.detachedArgs()); diff != "" {
				t.Errorf("detachedArgs (-want +got):\n%s", diff)
			}
		})
	}
}
