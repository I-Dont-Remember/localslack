package capabilities

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Fixture sources: where a fixture's expected response came from.
const (
	SourceLLM      = "llm"      // written from latent model knowledge, unchecked
	SourceDocs     = "docs"     // checked against a cited source (Ref)
	SourceRecorded = "recorded" // a sanitized real Slack response (Recorded holds provenance)
	SourceLocal    = "local"    // LocalSlack-only behavior (no Slack equivalent to verify)
)

// FixtureRequest is one HTTP request in a fixture.
type FixtureRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Token  string            `json:"token,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// SetupStep is a request run before the fixture's own; Capture maps a name to a
// dotted path in the response, usable as {{name}} in later paths and params.
type SetupStep struct {
	FixtureRequest
	Capture map[string]string `json:"capture,omitempty"`
}

// FixtureResponse is the expected response.
type FixtureResponse struct {
	Status int            `json:"status"`
	Body   map[string]any `json:"body"`
	Ignore []string       `json:"ignore,omitempty"`
}

// Recorded is the provenance of a fixture captured from real Slack.
type Recorded struct {
	Date          string   `json:"date"`           // YYYY-MM-DD
	Tool          string   `json:"tool"`           // e.g. record.py
	WorkspaceType string   `json:"workspace_type"` // free | pro | enterprise | sandbox
	TokenType     string   `json:"token_type"`     // bot | user
	Scopes        []string `json:"scopes,omitempty"`
	// Headers holds the response headers worth keeping (x-oauth-scopes, retry-after, ...).
	Headers map[string]string `json:"headers,omitempty"`
	Notes   string            `json:"notes,omitempty"`
}

// Fixture is a replayable request/response pair with provenance.
type Fixture struct {
	State    string          `json:"state,omitempty"` // passing (default) | known_gap
	Note     string          `json:"note,omitempty"`
	Source   string          `json:"source"`
	Ref      string          `json:"ref,omitempty"` // citation, required for source=docs
	Recorded *Recorded       `json:"recorded,omitempty"`
	GapDiffs []string        `json:"gap_diffs,omitempty"`
	Setup    []SetupStep     `json:"setup,omitempty"`
	Request  FixtureRequest  `json:"request"`
	Response FixtureResponse `json:"response"`
}

// NamedFixture pairs a fixture with its file name (without .json).
type NamedFixture struct {
	Name    string
	Fixture Fixture
}

// Method returns the Slack method a fixture exercises ("" for non-/api/ requests).
func (f Fixture) Method() string {
	name, ok := strings.CutPrefix(f.Request.Path, "/api/")
	if !ok || name == "" {
		return ""
	}
	return name
}

// ParseFixture decodes a fixture, rejecting unknown fields so typos are caught.
func ParseFixture(data []byte) (Fixture, error) {
	var f Fixture
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Fixture{}, fmt.Errorf("fixture json: %w", err)
	}
	return f, nil
}

// MarshalFixture renders a fixture as indented JSON with a trailing newline,
// leaving matchers like "<ts>" unescaped so files stay readable and diffable.
func MarshalFixture(f Fixture) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("marshal fixture: %w", err)
	}
	return buf.Bytes(), nil
}

// LoadFixtures reads every *.json file in the root of fsys, sorted by name.
func LoadFixtures(fsys fs.FS) ([]NamedFixture, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read fixtures: %w", err)
	}
	var out []NamedFixture
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		f, err := ParseFixture(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, NamedFixture{Name: strings.TrimSuffix(e.Name(), ".json"), Fixture: f})
	}
	return out, nil
}

// ValidateFixture returns problems with a fixture's schema and provenance.
func ValidateFixture(f Fixture) []string {
	var p problems
	switch f.Source {
	case "":
		p.addf("source is required (llm|docs|recorded|local)")
	case SourceLLM, SourceLocal:
	case SourceDocs:
		if strings.TrimSpace(f.Ref) == "" {
			p.addf("source docs requires a ref (citation: URL or source path)")
		}
	case SourceRecorded:
		validateRecorded(&p, f.Recorded)
	default:
		p.addf("invalid source %q (llm|docs|recorded|local)", f.Source)
	}
	if f.Source != SourceRecorded && f.Recorded != nil {
		p.addf("recorded provenance is only allowed when source is recorded")
	}
	if !slices.Contains([]string{"", "passing", "known_gap"}, f.State) {
		p.addf("invalid state %q (passing|known_gap)", f.State)
	}
	if f.Request.Path == "" || f.Request.Method == "" {
		p.addf("request method and path are required")
	}
	return p
}

func validateRecorded(p *problems, r *Recorded) {
	if r == nil {
		p.addf("source recorded requires a recorded{} provenance block")
		return
	}
	for field, v := range map[string]string{"date": r.Date, "tool": r.Tool, "workspace_type": r.WorkspaceType, "token_type": r.TokenType} {
		if strings.TrimSpace(v) == "" {
			p.addf("recorded.%s is required", field)
		}
	}
}
