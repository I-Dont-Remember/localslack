package capabilities

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// EvidenceNone means a method has no conformance fixture at all.
const EvidenceNone Evidence = "none"

// Rank orders evidence levels: none < llm < docs < recorded.
func Rank(e Evidence) int {
	switch e {
	case EvidenceLLM:
		return 1
	case EvidenceDocs:
		return 2
	case EvidenceRecorded:
		return 3
	}
	return 0
}

// EvidenceByMethod derives each Slack method's evidence level from its fixtures:
// the strongest source wins. Local-only fixtures prove nothing about real Slack,
// so they never count.
func EvidenceByMethod(fixtures []NamedFixture) map[string]Evidence {
	out := map[string]Evidence{}
	for _, nf := range fixtures {
		method := nf.Fixture.Method()
		if method == "" {
			continue
		}
		var e Evidence
		switch nf.Fixture.Source {
		case SourceRecorded:
			e = EvidenceRecorded
		case SourceDocs:
			e = EvidenceDocs
		case SourceLLM:
			e = EvidenceLLM
		default:
			continue
		}
		if Rank(e) > Rank(out[method]) {
			out[method] = e
		}
	}
	return out
}

// SuccessMethods reports which methods have at least one fixture whose response is ok:true.
// Without one, the evidence level only covers error paths.
func SuccessMethods(fixtures []NamedFixture) map[string]bool {
	out := map[string]bool{}
	for _, nf := range fixtures {
		if ok, _ := nf.Fixture.Response.Body["ok"].(bool); ok && nf.Fixture.Method() != "" {
			out[nf.Fixture.Method()] = true
		}
	}
	return out
}

// sdkTestDirs are scanned for real Slack SDK calls.
var sdkTestDirs = []string{"tests/smoke", "tests/e2e"} //nolint:gochecknoglobals // immutable

// scanTests walks the Python test dirs and calls visit for every .py file.
func scanTests(repo fs.FS, visit func(path string, src []byte)) error {
	for _, dir := range sdkTestDirs {
		err := fs.WalkDir(repo, dir, func(p string, d fs.DirEntry, err error) error {
			switch {
			case err != nil && p == dir:
				return nil // directory absent: nothing to scan
			case err != nil:
				return err
			case d.IsDir() && slices.Contains([]string{".venv", "__pycache__", "node_modules"}, d.Name()):
				return fs.SkipDir
			case d.IsDir() || path.Ext(p) != ".py":
				return nil
			}
			src, err := fs.ReadFile(repo, p)
			if err != nil {
				return err
			}
			visit(p, src)
			return nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", dir, err)
		}
	}
	return nil
}

// SDKTests finds which test files call each method through a Slack SDK client.
// The SDK names a method by replacing "." with "_" (chat.postMessage ->
// client.chat_postMessage(...)), so this needs no hand-kept list.
func SDKTests(repo fs.FS, methods []string) (map[string][]string, error) {
	patterns := make(map[string]*regexp.Regexp, len(methods))
	for _, m := range methods {
		patterns[m] = regexp.MustCompile(`\.` + regexp.QuoteMeta(strings.ReplaceAll(m, ".", "_")) + `\(`)
	}
	out := map[string][]string{}
	err := scanTests(repo, func(p string, src []byte) {
		for m, re := range patterns {
			if re.Match(src) {
				out[m] = append(out[m], p)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	for m := range out {
		slices.Sort(out[m])
	}
	return out, nil
}

var deviationMarker = regexp.MustCompile(`(?m)^\s*@deviation\(["']([a-z0-9-]+)["']\)`) //nolint:gochecknoglobals // immutable

// DeviationTests finds red tests tagged @deviation("<id>"): the executable record of a known
// deviation. Returns deviation id -> test files, sorted.
func DeviationTests(repo fs.FS) (map[string][]string, error) {
	out := map[string][]string{}
	err := scanTests(repo, func(p string, src []byte) {
		seen := map[string]bool{}
		for _, m := range deviationMarker.FindAllSubmatch(src, -1) {
			id := string(m[1])
			if !seen[id] {
				seen[id] = true
				out[id] = append(out[id], p)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out, nil
}

// Client is a client library version pinned by the smoke suite's lockfile.
type Client struct{ Name, Version string }

// ClientVersions reads the versions of the named packages from a uv.lock;
// a package that is not locked yet has an empty Version.
func ClientVersions(lock []byte, names ...string) ([]Client, error) {
	var parsed struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(lock, &parsed); err != nil {
		return nil, fmt.Errorf("parse uv.lock: %w", err)
	}
	versions := map[string]string{}
	for _, p := range parsed.Package {
		versions[p.Name] = p.Version
	}
	out := make([]Client, 0, len(names))
	for _, n := range names {
		out = append(out, Client{Name: n, Version: versions[n]})
	}
	return out, nil
}

// Report is the manifest plus everything derived from the repository.
type Report struct {
	Manifest Manifest
	Evidence map[string]Evidence // per method
	Success  map[string]bool     // per method: has an ok:true fixture
	SDK      map[string][]string // per method: test files calling it
	DevTests map[string][]string // per deviation: red tests tagged @deviation(id)
	Clients  []Client
}

// BuildReport loads the embedded manifest and derives evidence, SDK coverage and
// client versions from the repository rooted at repo.
func BuildReport(repo fs.FS) (Report, error) {
	m, err := Load()
	if err != nil {
		return Report{}, err
	}
	fixDir, err := fs.Sub(repo, "tests/conformance/fixtures")
	if err != nil {
		return Report{}, fmt.Errorf("fixtures dir: %w", err)
	}
	fixtures, err := LoadFixtures(fixDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // no fixtures yet is a valid (all "none") state
		return Report{}, err
	}
	sdk, err := SDKTests(repo, sortedKeys(m.Methods))
	if err != nil {
		return Report{}, err
	}
	devTests, err := DeviationTests(repo)
	if err != nil {
		return Report{}, err
	}
	var clients []Client
	if lock, err := fs.ReadFile(repo, "tests/smoke/uv.lock"); err == nil {
		if clients, err = ClientVersions(lock, "slack-sdk", "slack-bolt", "aiohttp"); err != nil {
			return Report{}, err
		}
	}
	return Report{Manifest: m, Evidence: EvidenceByMethod(fixtures), Success: SuccessMethods(fixtures), SDK: sdk, DevTests: devTests, Clients: clients}, nil
}

// Gaps lists shipped methods whose evidence is below "recorded", weakest first:
// the to-do list for anyone with a workspace to record from. Methods marked
// verify = "unlikely" are left out.
func (r Report) Gaps() []string {
	var names []string
	for name, meth := range r.Manifest.Methods {
		if shipped(meth.Status) && meth.Verify != VerifyUnlikely && r.level(name) != EvidenceRecorded {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		if d := Rank(r.level(a)) - Rank(r.level(b)); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return names
}

func (r Report) level(method string) Evidence {
	if e, ok := r.Evidence[method]; ok {
		return e
	}
	return EvidenceNone
}

// levelLabel is the evidence cell: the level, flagged when no success-path fixture backs it.
func (r Report) levelLabel(method string) string {
	l := r.level(method)
	label := string(l)
	if l != EvidenceNone && !r.Success[method] {
		label += " (errors only)"
	}
	if r.Manifest.Methods[method].Verify == VerifyUnlikely {
		label += " (recording not expected)"
	}
	return label
}
