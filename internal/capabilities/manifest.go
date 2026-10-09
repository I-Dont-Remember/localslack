// Package capabilities is the single, test-enforced inventory of what LocalSlack
// supports: Slack methods, non-endpoint behaviors, and known deviations from real
// Slack. The hand-edited source is capabilities.toml; evidence levels, SDK-test
// coverage and docs/COMPATIBILITY.md are derived from it (see derive.go, render.go).
package capabilities

import (
	"bytes"
	_ "embed" // capabilities.toml
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed capabilities.toml
var embedded []byte

// Status is where a method or behavior is in its lifecycle.
type Status string

// Lifecycle statuses.
const (
	StatusPlanned     Status = "planned"
	StatusInProgress  Status = "in_progress"
	StatusPartial     Status = "partial"
	StatusImplemented Status = "implemented"
)

// Evidence is how a behavior's real-Slack fidelity was established.
type Evidence string

// Evidence levels, weakest to strongest.
const (
	EvidenceLLM      Evidence = "llm"      // latent model knowledge, unchecked
	EvidenceDocs     Evidence = "docs"     // checked against a cited source (Slack docs, SDK source)
	EvidenceRecorded Evidence = "recorded" // a real Slack response is committed
)

// Method describes one Slack Web API method.
type Method struct {
	Status     Status   `toml:"status"`
	Tier       int      `toml:"tier"` // Slack rate-limit tier 1-4; 0 = special/unset
	Scopes     []string `toml:"scopes"`
	Deviations []string `toml:"deviations"`
	Priority   string   `toml:"priority"`
	Plan       string   `toml:"plan"`     // required while in_progress
	Fixtures   string   `toml:"fixtures"` // "waived: reason" opts out of the fixture requirement
}

// Behavior describes a non-endpoint capability or HTTP-level quirk.
type Behavior struct {
	Status     Status   `toml:"status"`
	Summary    string   `toml:"summary"`
	Evidence   Evidence `toml:"evidence"`
	Ref        string   `toml:"ref"` // citation (URL, SDK source path or fixture) for docs/recorded evidence
	Milestone  string   `toml:"milestone"`
	Deviations []string `toml:"deviations"`
}

// Deviation is a documented way LocalSlack differs from real Slack.
type Deviation struct {
	Summary  string `toml:"summary"`
	Severity string `toml:"severity"` // high | medium | low
}

// Manifest is the parsed capabilities file.
type Manifest struct {
	Methods    map[string]Method    `toml:"methods"`
	Behaviors  map[string]Behavior  `toml:"behaviors"`
	Deviations map[string]Deviation `toml:"deviations"`
}

// Parse decodes a manifest, rejecting unknown fields so typos are not silently ignored.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	dec := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return Manifest{}, fmt.Errorf("capabilities toml: unknown field(s):\n%s", strict.String())
		}
		return Manifest{}, fmt.Errorf("capabilities toml: %w", err)
	}
	return m, nil
}

// Load parses the embedded capabilities.toml.
func Load() (Manifest, error) { return Parse(embedded) }

func validStatus(s Status) bool {
	return slices.Contains([]Status{StatusPlanned, StatusInProgress, StatusPartial, StatusImplemented}, s)
}

func shipped(s Status) bool { return s == StatusImplemented || s == StatusPartial }

// Validate returns human-readable problems (empty when the manifest is sound).
// repo is the repository root, used to check that referenced plan files exist.
func Validate(m Manifest, repo fs.FS) []string {
	var p problems
	for _, name := range sortedKeys(m.Methods) {
		validateMethod(&p, m, repo, name, m.Methods[name])
	}
	for _, name := range sortedKeys(m.Behaviors) {
		validateBehavior(&p, m, name, m.Behaviors[name])
	}
	for _, name := range sortedKeys(m.Deviations) {
		d := m.Deviations[name]
		if strings.TrimSpace(d.Summary) == "" {
			p.addf("deviation %q: summary is required", name)
		}
		if !slices.Contains([]string{"high", "medium", "low"}, d.Severity) {
			p.addf("deviation %q: invalid severity %q (high|medium|low)", name, d.Severity)
		}
	}
	return p
}

type problems []string

func (p *problems) addf(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }

func validateMethod(p *problems, m Manifest, repo fs.FS, name string, meth Method) {
	switch {
	case meth.Status == "":
		p.addf("method %q: status is required (planned|in_progress|partial|implemented)", name)
	case !validStatus(meth.Status):
		p.addf("method %q: invalid status %q", name, meth.Status)
	}
	if meth.Tier < 0 || meth.Tier > 4 {
		p.addf("method %q: tier %d out of range 0-4 (0 = special or unset)", name, meth.Tier)
	}
	if meth.Status == StatusInProgress {
		if meth.Plan == "" {
			p.addf("method %q: in_progress requires a plan (path to the plan file that owns the work)", name)
		} else if _, err := fs.Stat(repo, meth.Plan); err != nil {
			p.addf("method %q: plan file %q does not exist", name, meth.Plan)
		}
	}
	checkDeviationRefs(p, m, "method", name, meth.Deviations)
}

func validateBehavior(p *problems, m Manifest, name string, b Behavior) {
	if !validStatus(b.Status) {
		p.addf("behavior %q: invalid status %q", name, b.Status)
	}
	if strings.TrimSpace(b.Summary) == "" {
		p.addf("behavior %q: summary is required", name)
	}
	if shipped(b.Status) && b.Evidence == "" {
		p.addf("behavior %q: evidence (llm|docs|recorded) is required once implemented or partial", name)
	}
	if b.Evidence != "" && !slices.Contains([]Evidence{EvidenceLLM, EvidenceDocs, EvidenceRecorded}, b.Evidence) {
		p.addf("behavior %q: invalid evidence %q", name, b.Evidence)
	}
	if (b.Evidence == EvidenceDocs || b.Evidence == EvidenceRecorded) && strings.TrimSpace(b.Ref) == "" {
		p.addf("behavior %q: ref (citation) is required for evidence %q", name, b.Evidence)
	}
	checkDeviationRefs(p, m, "behavior", name, b.Deviations)
}

func checkDeviationRefs(p *problems, m Manifest, kind, name string, refs []string) {
	for _, d := range refs {
		if _, ok := m.Deviations[d]; !ok {
			p.addf("%s %q: unknown deviation %q (define it under [deviations])", kind, name, d)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
