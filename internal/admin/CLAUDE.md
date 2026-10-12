# CLAUDE.md

`/_localslack/*` is our own API: real HTTP statuses (2xx resource body, 4xx/5xx `{"error":{"code","message"}}`), never an `ok` wrapper or 200 + error.
Write responses only via `respond.JSON`/`respond.Error`, `writeError`, `writeNoContent` (admin.go); no `http.Error`, `.WriteHeader(`, `json.NewEncoder(`, or `"ok"` literal (ast-grep `admin-*` rules).
Every route must be in `docs/TESTING_API.md`; `internal/server/route_contract_test.go` fails on undocumented routes, wrong error shapes, and missing success probes (add one there for new routes).
