package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"localslack/internal/xdg"
)

// Note: t.Setenv and t.Parallel are incompatible in Go — these tests run
// sequentially, but the test binary is shuffled via -shuffle=on.

func TestDataHome(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			name:   "default when unset",
			envVal: "",
			want:   filepath.Join(os.Getenv("HOME"), ".local", "share"),
		},
		{
			name:   "custom via env",
			envVal: "/custom/data",
			want:   "/custom/data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", tt.envVal)
			got := xdg.DataHome()
			if got != tt.want {
				t.Errorf("DataHome() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeDir(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			name:   "default when unset",
			envVal: "",
			want:   filepath.Join(os.Getenv("HOME"), ".local", "run"),
		},
		{
			name:   "custom via env",
			envVal: "/custom/runtime",
			want:   "/custom/runtime",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", tt.envVal)
			got := xdg.RuntimeDir()
			if got != tt.want {
				t.Errorf("RuntimeDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test/data")
	got := xdg.AppDataDir()
	want := "/test/data/localslack"
	if got != want {
		t.Errorf("AppDataDir() = %q, want %q", got, want)
	}
}

func TestAppRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/test/runtime")
	got := xdg.AppRuntimeDir()
	want := "/test/runtime/localslack"
	if got != want {
		t.Errorf("AppRuntimeDir() = %q, want %q", got, want)
	}
}

func TestPIDFile(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/test/runtime")
	got := xdg.PIDFile()
	want := "/test/runtime/localslack/localslack.pid"
	if got != want {
		t.Errorf("PIDFile() = %q, want %q", got, want)
	}
}
