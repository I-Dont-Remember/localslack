package xdg_test

import (
	"testing"

	"localslack/internal/xdg"
)

func lookup(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func TestDirs(t *testing.T) {
	t.Parallel()
	home := map[string]string{"HOME": "/h"}
	custom := map[string]string{
		"HOME":            "/h",
		"XDG_DATA_HOME":   "/c/data",
		"XDG_STATE_HOME":  "/c/state",
		"XDG_RUNTIME_DIR": "/c/run",
	}
	tests := []struct {
		name string
		env  map[string]string
		fn   func(xdg.Dirs) string
		want string
	}{
		{"DataHome default", home, xdg.Dirs.DataHome, "/h/.local/share"},
		{"DataHome custom", custom, xdg.Dirs.DataHome, "/c/data"},
		{"StateHome default", home, xdg.Dirs.StateHome, "/h/.local/state"},
		{"StateHome custom", custom, xdg.Dirs.StateHome, "/c/state"},
		{"RuntimeDir default", home, xdg.Dirs.RuntimeDir, "/h/.local/run"},
		{"RuntimeDir custom", custom, xdg.Dirs.RuntimeDir, "/c/run"},
		{"AppDataDir default", home, xdg.Dirs.AppDataDir, "/h/.local/share/localslack"},
		{"AppDataDir custom", custom, xdg.Dirs.AppDataDir, "/c/data/localslack"},
		{"AppStateDir default", home, xdg.Dirs.AppStateDir, "/h/.local/state/localslack"},
		{"AppStateDir custom", custom, xdg.Dirs.AppStateDir, "/c/state/localslack"},
		{"PIDFile default", home, xdg.Dirs.PIDFile, "/h/.local/state/localslack/localslack.pid"},
		{"PIDFile custom", custom, xdg.Dirs.PIDFile, "/c/state/localslack/localslack.pid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.fn(xdg.New(lookup(tt.env)))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
