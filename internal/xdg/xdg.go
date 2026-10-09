// Package xdg provides XDG Base Directory paths for LocalSlack.
// The Linux XDG defaults are used on all platforms (including macOS)
// for consistency. Do not construct these paths manually — use these helpers.
package xdg

import (
	"os"
	"path/filepath"
)

// Dirs resolves XDG paths from an environment lookup, so tests need not touch the process env.
type Dirs struct {
	getenv func(string) string
}

// New returns Dirs backed by getenv.
func New(getenv func(string) string) Dirs { return Dirs{getenv: getenv} }

func system() Dirs { return New(os.Getenv) }

func (d Dirs) fromEnv(key string, fallback ...string) string {
	if dir := d.getenv(key); dir != "" {
		return dir
	}
	return filepath.Join(append([]string{d.getenv("HOME")}, fallback...)...)
}

// DataHome returns XDG_DATA_HOME, defaulting to $HOME/.local/share.
func (d Dirs) DataHome() string { return d.fromEnv("XDG_DATA_HOME", ".local", "share") }

// StateHome returns XDG_STATE_HOME, defaulting to $HOME/.local/state.
// State data persists across restarts but is not user-facing (PID files, logs, etc.).
func (d Dirs) StateHome() string { return d.fromEnv("XDG_STATE_HOME", ".local", "state") }

// RuntimeDir returns XDG_RUNTIME_DIR, defaulting to $HOME/.local/run.
// For session-scoped files only (sockets, named pipes) — cleared on logout.
func (d Dirs) RuntimeDir() string { return d.fromEnv("XDG_RUNTIME_DIR", ".local", "run") }

// AppDataDir returns the LocalSlack data directory (DB files, etc.).
func (d Dirs) AppDataDir() string { return filepath.Join(d.DataHome(), "localslack") }

// AppStateDir returns the LocalSlack state directory (PID files, logs, etc.).
func (d Dirs) AppStateDir() string { return filepath.Join(d.StateHome(), "localslack") }

// PIDFile returns the path to the LocalSlack PID file.
func (d Dirs) PIDFile() string { return filepath.Join(d.AppStateDir(), "localslack.pid") }

// DataHome returns the process's XDG data home.
func DataHome() string { return system().DataHome() }

// StateHome returns the process's XDG state home.
func StateHome() string { return system().StateHome() }

// RuntimeDir returns the process's XDG runtime dir.
func RuntimeDir() string { return system().RuntimeDir() }

// AppDataDir returns the process's LocalSlack data directory.
func AppDataDir() string { return system().AppDataDir() }

// AppStateDir returns the process's LocalSlack state directory.
func AppStateDir() string { return system().AppStateDir() }

// PIDFile returns the process's LocalSlack PID file path.
func PIDFile() string { return system().PIDFile() }
