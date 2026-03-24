// Package xdg provides XDG Base Directory paths for LocalSlack.
// The Linux XDG defaults are used on all platforms (including macOS)
// for consistency. Do not construct these paths manually — use these helpers.
package xdg

import (
	"os"
	"path/filepath"
)

// DataHome returns XDG_DATA_HOME, defaulting to $HOME/.local/share.
func DataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

// StateHome returns XDG_STATE_HOME, defaulting to $HOME/.local/state.
// State data persists across restarts but is not user-facing (PID files, logs, etc.).
func StateHome() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state")
}

// RuntimeDir returns XDG_RUNTIME_DIR, defaulting to $HOME/.local/run.
// For session-scoped files only (sockets, named pipes) — cleared on logout.
func RuntimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "run")
}

// AppDataDir returns the LocalSlack data directory (DB files, etc.).
func AppDataDir() string {
	return filepath.Join(DataHome(), "localslack")
}

// AppStateDir returns the LocalSlack state directory (PID files, logs, etc.).
func AppStateDir() string {
	return filepath.Join(StateHome(), "localslack")
}

// PIDFile returns the path to the LocalSlack PID file.
func PIDFile() string {
	return filepath.Join(AppStateDir(), "localslack.pid")
}
