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

// RuntimeDir returns XDG_RUNTIME_DIR, defaulting to $HOME/.local/run.
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

// AppRuntimeDir returns the LocalSlack runtime directory (PID files, sockets).
func AppRuntimeDir() string {
	return filepath.Join(RuntimeDir(), "localslack")
}

// PIDFile returns the path to the LocalSlack PID file.
func PIDFile() string {
	return filepath.Join(AppRuntimeDir(), "localslack.pid")
}
