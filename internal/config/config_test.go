package config_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/config"
)

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
			want: config.Config{Addr: ":3232", DBPath: "/h/.local/share/localslack/localslack.db"},
		},
		{
			name: "xdg data home",
			env:  map[string]string{"XDG_DATA_HOME": "/x"},
			want: config.Config{Addr: ":3232", DBPath: "/x/localslack/localslack.db"},
		},
		{
			name: "overrides",
			env: map[string]string{
				"LOCALSLACK_ADDR":    "127.0.0.1:9",
				"LOCALSLACK_DB_PATH": "/tmp/a.db",
				"LOCALSLACK_SEED":    "42",
				"LOCALSLACK_NO_SEED": "1",
			},
			want: config.Config{Addr: "127.0.0.1:9", DBPath: "/tmp/a.db", Seed: "42", NoSeed: true},
		},
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
