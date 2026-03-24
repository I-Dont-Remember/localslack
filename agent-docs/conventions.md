# Project Conventions

## File Paths — XDG Base Directory

All runtime and data files follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). The same Linux defaults are used on macOS — no platform branching.

| Purpose | Variable | Default |
|---------|----------|---------|
| Data (DB, etc.) | `XDG_DATA_HOME` | `$HOME/.local/share/localslack` |
| Runtime (PID, sockets) | `XDG_RUNTIME_DIR` | `$HOME/.local/run/localslack` |

**Always use `internal/xdg/` helpers — never construct these paths manually.**
