# Coding Standards - Go

- write clean, idiomatic Go code following best practices

## Key Reminders

Follow the conventions and patterns documented at https://go.dev/doc/effective_go, with particular attention to:

- Formatting: Always use gofmt - this is non-negotiable
- Naming: No underscores, use MixedCaps for exported names, mixedCaps for unexported
- Error handling: Always check errors; return them, don't panic
- Concurrency: Share memory by communicating (use channels)
- Interfaces: Keep small (1-3 methods ideal); accept interfaces, return concrete types
- Use all automated tools at your disposal for high-quality code: formatters, linters, vet, golangci-lint, performance checks, etc. Don't rely on your first pass implementation being perfect with minor details, let the automated checks help you.

## Design Patterns

### Data Structures

- Avoid putting logic, registration side-effects, or configuration loading in `init` - prefer explicit functions

### Validation

- Accept `context.Context` as first parameter in functions that perform I/O, RPC, network, or long-running operations
- Do not store context in structs or pass nil; propagate the provided context downstream
- Use context for request-scoped values and cancellation signals; avoid using as global variable store

### State Management

- Use dependency injection patterns where appropriate for testability and modularity

### Logging

- Use structured logging with key-value pairs rather than printf-style scattering
- use the `slog` package; avoid global logger, created dedicated `slog.Logger` instances
- Use `slog.Attr` (e.g. `slog.String("key", value)`) over key-value pairs for type safety
- Use static messages, provide context via attributes not string formatting in the message
- Include correlation identifiers only when already available (do NOT fabricate IDs)
- Use the context-aware logger methods whenever a `context.Context` is available (e.g. `InfoContext` instead of `Info`)

### Imports & Module Management

- Use minimal necessary dependencies; use standard library
- If a library is to be used, use a lightweight one that is well-maintained
- Avoid dot imports; avoid blank imports outside main packages or tests

### Error Handling

- Avoid ignoring errors unless explicitly safe; if ignored, add a comment explaining why (`_ =`)
- Do NOT overuse panic; restrict it to only truly unrecoverable errors or impossible states
- Handle each error only once: either log OR return, not both (avoid duplicate handling multiple times in the call stack)
- Keep happy path to the left; handle errors early and return to reduce nesting

### Tests

- Keep tests deterministic; avoid `time.Now` wihtout injection or control
- Use table-driven tests when variation matters
- Avoid over-mocking; use real small units or fakes
- Use dependency injection instead of monkey patching
- Use want/got instead of expected/actual when naming identifiers in tests
- Use `t.Setenv()` instead of `os.Setenv()` to ensure cleanup
- Prefer `t.Error` over `t.Fatal` to allow tests to continue and report all failures
- Structure all tests clearly with setup, execution, and assertion phases; make them readable

### Documentation

- Provide short focused comments; no verbose narratives in .go files
- Include examples in documentation for complex APIs
- Write examples as `Example` functions instead of doc comments

### Analysis

- Analyze code for runtime errors or panics
- Analyze code for potential performance issues, e.g. unneccessary allocations or inefficient algorithms

## Custom lint rules (ast-grep)

Repo-specific bad patterns live in `rules/*.yml` (run by `make lint`, pre-commit). Each rule has a test in `rule-tests/`.
- Add a rule when you catch a recurring mistake. Run `ast-grep test --update-all` to refresh snapshots, then review them.
- Gotcha: a bare `pkg.Func($$$)` pattern parses as a type conversion in Go. Use `pattern: {context: 'func _() { pkg.Func($$$) }', selector: call_expression}`, and wrap test snippets in `func f() { ... }`.
- ast-grep skips gitignored paths (e.g. `tmp/`).
