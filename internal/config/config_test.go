package config_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/config"
)

// withDefaults fills the test-loop defaults into c so cases state only what differs.
func withDefaults(c config.Config) config.Config {
	c.EventTimeout = 3 * time.Second
	c.EventRetryDelays = []time.Duration{0, time.Minute, 5 * time.Minute}
	c.CallLogSize = 10000
	c.DeliveryLogSize = 2000
	return c
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		env     map[string]string
		want    config.Config
		wantErr bool
	}{
		{
			name: "defaults",
			env:  map[string]string{"HOME": "/h"},
			want: withDefaults(config.Config{Addr: "127.0.0.1:3232", DBPath: "/h/.local/share/localslack/localslack.db"}),
		},
		{
			name: "xdg data home",
			env:  map[string]string{"XDG_DATA_HOME": "/x"},
			want: withDefaults(config.Config{Addr: "127.0.0.1:3232", DBPath: "/x/localslack/localslack.db"}),
		},
		{
			name: "overrides",
			env: map[string]string{
				"LOCALSLACK_ADDR":    "127.0.0.1:9",
				"LOCALSLACK_DB_PATH": "/tmp/a.db",
				"LOCALSLACK_SEED":    "42",
				"LOCALSLACK_NO_SEED": "1",
			},
			want: withDefaults(config.Config{Addr: "127.0.0.1:9", DBPath: "/tmp/a.db", Seed: "42", NoSeed: true}),
		},
		{
			name: "explicit all-interfaces bind is honored",
			env:  map[string]string{"LOCALSLACK_ADDR": "0.0.0.0:3232", "HOME": "/h"},
			want: withDefaults(config.Config{Addr: "0.0.0.0:3232", DBPath: "/h/.local/share/localslack/localslack.db"}),
		},
		{
			name: "test-loop settings",
			env: map[string]string{
				"LOCALSLACK_ADDR":                      "127.0.0.1:9",
				"LOCALSLACK_DB_PATH":                   "/tmp/a.db",
				"LOCALSLACK_APP_MANIFEST":              "a.json, b.yaml",
				"LOCALSLACK_EVENT_TIMEOUT":             "500ms",
				"LOCALSLACK_EVENT_RETRY_DELAYS":        "0s,100ms,200ms",
				"LOCALSLACK_SCENARIO":                  "acme.json",
				"LOCALSLACK_CALL_LOG_SIZE":             "5",
				"LOCALSLACK_DELIVERY_LOG_SIZE":         "6",
				"LOCALSLACK_STRICT_TOKENS":             "true",
				"LOCALSLACK_DELIVER_TO":                "http://localhost:3000",
				"LOCALSLACK_ALLOW_REMOTE_REQUEST_URLS": "1",
			},
			want: config.Config{
				Addr: "127.0.0.1:9", DBPath: "/tmp/a.db",
				AppManifest: []string{"a.json", "b.yaml"}, EventTimeout: 500 * time.Millisecond,
				EventRetryDelays: []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond},
				Scenario:         "acme.json", CallLogSize: 5, DeliveryLogSize: 6, StrictTokens: true,
				DeliverTo: "http://localhost:3000", AllowRemoteRequestURLs: true,
			},
		},
		{
			name: "none disables retries",
			env:  map[string]string{"LOCALSLACK_DB_PATH": "/d.db", "LOCALSLACK_EVENT_RETRY_DELAYS": "none"},
			want: func() config.Config {
				c := withDefaults(config.Config{Addr: "127.0.0.1:3232", DBPath: "/d.db"})
				c.EventRetryDelays = nil
				return c
			}(),
		},
		{name: "bad timeout", env: map[string]string{"LOCALSLACK_EVENT_TIMEOUT": "soon"}, wantErr: true},
		{name: "bad retry delays", env: map[string]string{"LOCALSLACK_EVENT_RETRY_DELAYS": "0s,x"}, wantErr: true},
		{name: "negative delay", env: map[string]string{"LOCALSLACK_EVENT_RETRY_DELAYS": "-1s"}, wantErr: true},
		{name: "bad log size", env: map[string]string{"LOCALSLACK_CALL_LOG_SIZE": "-3"}, wantErr: true},
		{name: "bad allow remote", env: map[string]string{"LOCALSLACK_ALLOW_REMOTE_REQUEST_URLS": "perhaps"}, wantErr: true},
		{name: "bad strict tokens", env: map[string]string{"LOCALSLACK_STRICT_TOKENS": "perhaps"}, wantErr: true},
		{
			name:    "bad no-seed value",
			env:     map[string]string{"LOCALSLACK_NO_SEED": "maybe"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := config.Load(func(k string) string { return tt.env[k] })
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() err = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Load() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
