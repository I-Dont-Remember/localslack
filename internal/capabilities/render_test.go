package capabilities_test

import (
	"os"
	"strings"
	"testing"

	"localslack/internal/capabilities"
)

func smallReport(t *testing.T) capabilities.Report {
	t.Helper()
	m, err := capabilities.Parse([]byte(`
[methods."a.shipped"]
status = "implemented"
tier = 2
scopes = ["x:read"]
deviations = ["gap"]
[methods."b.partial"]
status = "partial"
[methods."c.planned"]
status = "planned"
priority = "bolt-core"
[behaviors.rate-limiting]
status = "planned"
milestone = "M5"
summary = "Per-method rate limits."
[deviations.gap]
severity = "high"
summary = "Something differs."
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return capabilities.Report{
		Manifest: m,
		Evidence: map[string]capabilities.Evidence{"a.shipped": capabilities.EvidenceRecorded},
		SDK:      map[string][]string{"a.shipped": {"tests/smoke/a_test.py"}},
		Clients:  []capabilities.Client{{Name: "slack-sdk", Version: "3.41.0"}, {Name: "slack-bolt"}},
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	out := capabilities.Render(smallReport(t))
	for _, want := range []string{
		"Generated from", // do-not-edit banner
		"| `a.shipped` | implemented | recorded | 2 | `x:read` | yes |", // derived evidence + sdk-tested
		"| `b.partial` | partial | none |",                              // no fixtures: honest "none"
		"| `c.planned` | planned |",                                     // planned section present
		"bolt-core",                                                     // priority shown
		"rate-limiting",                                                 // behaviors table
		"M5",
		"Something differs.", // deviation catalogue
		"high",
		"slack-sdk | 3.41.0", // client versions
		"slack-bolt | not yet",
		"1 implemented, 1 partial, 1 planned", // summary line
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered doc missing %q:\n%s", want, out)
		}
	}
	if again := capabilities.Render(smallReport(t)); again != out {
		t.Error("Render is not deterministic")
	}
}

// docs/COMPATIBILITY.md is generated: if this fails, run `make docs` and commit the result.
func TestCompatibilityDocIsCurrent(t *testing.T) {
	t.Parallel()

	repo := os.DirFS("../..")
	r, err := capabilities.BuildReport(repo)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	got := capabilities.Render(r)
	want, err := os.ReadFile("../../docs/COMPATIBILITY.md")
	if err != nil {
		t.Fatalf("read docs/COMPATIBILITY.md (run `make docs`): %v", err)
	}
	if string(want) != got {
		t.Error("docs/COMPATIBILITY.md is stale or hand-edited; run `make docs` and commit the result")
	}
}
