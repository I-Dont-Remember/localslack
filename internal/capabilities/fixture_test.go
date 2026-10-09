package capabilities_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"localslack/internal/capabilities"
)

func fixtureJSON(source, extra string) string {
	return `{"state":"passing",` + source + extra + `"request":{"method":"POST","path":"/api/auth.test"},"response":{"status":200,"body":{"ok":true}}}`
}

const recordedBlock = `"recorded":{"date":"2026-10-09","tool":"record.py","workspace_type":"free","token_type":"bot","scopes":["chat:write"]},`

func TestParseFixture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantErr string // substring of an expected problem or parse error; "" = valid
	}{
		{"llm ok", fixtureJSON(`"source":"llm",`, ""), ""},
		{"local ok", fixtureJSON(`"source":"local",`, ""), ""},
		{"docs with ref ok", fixtureJSON(`"source":"docs","ref":"https://docs.slack.dev/reference/methods/auth.test",`, ""), ""},
		{"recorded with provenance ok", fixtureJSON(`"source":"recorded",`, recordedBlock), ""},
		{"source is required", fixtureJSON("", ""), "source"},
		{"bad source", fixtureJSON(`"source":"vibes",`, ""), `"vibes"`},
		{"docs needs a ref", fixtureJSON(`"source":"docs",`, ""), "ref"},
		{"recorded needs provenance", fixtureJSON(`"source":"recorded",`, ""), "recorded"},
		{
			"recorded provenance must be complete",
			fixtureJSON(`"source":"recorded",`, `"recorded":{"tool":"record.py"},`),
			"date",
		},
		{"non-recorded must not carry provenance", fixtureJSON(`"source":"llm",`, recordedBlock), "recorded"},
		{"unknown field is a typo", fixtureJSON(`"source":"llm","gap_diff":["x"],`, ""), "gap_diff"},
		{"bad json", `{`, "json"},
		{"bad state", strings.Replace(fixtureJSON(`"source":"llm",`, ""), `"passing"`, `"mostly"`, 1), "state"},
		{"request path required", `{"source":"llm","request":{"method":"POST"},"response":{"status":200,"body":{}}}`, "path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, err := capabilities.ParseFixture([]byte(tt.json))
			var all string
			if err != nil {
				all = err.Error()
			} else {
				all = strings.Join(capabilities.ValidateFixture(f), "\n")
			}
			if tt.wantErr == "" && all != "" {
				t.Errorf("unexpected problems: %s", all)
			}
			if tt.wantErr != "" && !strings.Contains(all, tt.wantErr) {
				t.Errorf("problems %q missing %q", all, tt.wantErr)
			}
		})
	}
}

func TestFixtureMethod(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/api/conversations.history": "conversations.history",
		"/api/auth.test":             "auth.test",
		"/_localslack/health":        "",
		"/api/":                      "",
	}
	for path, want := range tests {
		f := capabilities.Fixture{Request: capabilities.FixtureRequest{Path: path}}
		if got := f.Method(); got != want {
			t.Errorf("Method(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLoadFixtures(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"a.json":    {Data: []byte(fixtureJSON(`"source":"llm",`, ""))},
		"b.json":    {Data: []byte(fixtureJSON(`"source":"local",`, ""))},
		"notes.txt": {Data: []byte("ignored")},
	}
	got, err := capabilities.LoadFixtures(fsys)
	if err != nil || len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("LoadFixtures = %+v, %v; want a, b", got, err)
	}
	fsys["c.json"] = &fstest.MapFile{Data: []byte(`{`)}
	if _, err := capabilities.LoadFixtures(fsys); err == nil || !strings.Contains(err.Error(), "c.json") {
		t.Errorf("LoadFixtures with bad file err = %v, want mention of c.json", err)
	}
}

func repoFixtures(t *testing.T) []capabilities.NamedFixture {
	t.Helper()
	fs, err := capabilities.LoadFixtures(os.DirFS("../../tests/conformance/fixtures"))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	return fs
}

func TestRepoFixturesAreValid(t *testing.T) {
	t.Parallel()

	for _, nf := range repoFixtures(t) {
		if problems := capabilities.ValidateFixture(nf.Fixture); len(problems) != 0 {
			t.Errorf("fixture %s: %s", nf.Name, strings.Join(problems, "; "))
		}
	}
}

// Every shipped method needs at least one conformance fixture, or an explicit waiver.
func TestEveryShippedMethodHasAFixture(t *testing.T) {
	t.Parallel()

	m, err := capabilities.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	have := map[string]bool{}
	for _, nf := range repoFixtures(t) {
		have[nf.Fixture.Method()] = true
	}
	for name, meth := range m.Methods {
		if (meth.Status == capabilities.StatusImplemented || meth.Status == capabilities.StatusPartial) &&
			!have[name] && !strings.HasPrefix(meth.Fixtures, "waived:") {
			t.Errorf("method %s is %s but has no fixture in tests/conformance/fixtures; add one or set fixtures = \"waived: reason\"", name, meth.Status)
		}
	}
}
