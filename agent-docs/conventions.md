# Project Conventions

## File Paths — XDG Base Directory

All runtime and data files follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). The same Linux defaults are used on macOS — no platform branching.

| Purpose | Variable | Default | Examples |
|---------|----------|---------|---------|
| Data (DB, etc.) | `XDG_DATA_HOME` | `$HOME/.local/share` | `AppDataDir()` |
| State (PID, logs) | `XDG_STATE_HOME` | `$HOME/.local/state` | `AppStateDir()`, `PIDFile()` |
| Runtime (sockets) | `XDG_RUNTIME_DIR` | `$HOME/.local/run` | `RuntimeDir()` |

`XDG_STATE_HOME` is often unset (e.g. WSL) — the default `$HOME/.local/state` is always used as fallback.

**Always use `internal/xdg/` helpers — never construct these paths manually.**
