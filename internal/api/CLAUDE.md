# CLAUDE.md

`/api/*` is Slack-fidelity: HTTP 200 with `{"ok":false,"error":code}` for method errors; real status only for 429, 5xx, and fault overrides. (The `/_localslack` convention is different: see `internal/admin/CLAUDE.md`.)
Write responses only via `api/respond.go` (`fail`, `write`); no `http.Error`, `.WriteHeader(`, `json.NewEncoder(` elsewhere (ast-grep `api-use-respond-helpers`).
