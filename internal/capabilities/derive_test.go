package capabilities_test

import (
	"os"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/capabilities"
)

func fx(source, path string) capabilities.NamedFixture {
	return capabilities.NamedFixture{Name: path, Fixture: capabilities.Fixture{
		Source: source, Request: capabilities.FixtureRequest{Method: "POST", Path: path},
	}}
}

func TestEvidenceByMethod(t *testing.T) {
	t.Parallel()

	got := capabilities.EvidenceByMethod([]capabilities.NamedFixture{
		fx("llm", "/api/a.one"),
		fx("recorded", "/api/a.one"), // strongest wins
		fx("docs", "/api/b.two"),
		fx("llm", "/api/b.two"),
		fx("llm", "/api/c.three"),
		fx("local", "/_localslack/health"), // not a Slack method
		fx("local", "/api/does.notExist"),  // LocalSlack-only: proves nothing about Slack
	})
	want := map[string]capabilities.Evidence{
		"a.one":   capabilities.EvidenceRecorded,
		"b.two":   capabilities.EvidenceDocs,
		"c.three": capabilities.EvidenceLLM,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("EvidenceByMethod mismatch (-want +got):\n%s", diff)
	}
}

func TestSDKTests(t *testing.T) {
	t.Parallel()

	repo := fstest.MapFS{
		"tests/smoke/a_test.py": {Data: []byte("def test_x(client):\n    client.conversations_history(channel=c)\n    client.auth_test()\n")},
		"tests/e2e/b_test.py":   {Data: []byte("await async_client.chat_postMessage(channel=c, text=t)\n")},
		// Not SDK calls: a test *named* after the method, a bare word, and the venv.
		"tests/smoke/c_test.py":                   {Data: []byte("def test_auth_test(client):\n    pass\n# users_list mentioned in a comment\n")},
		"tests/smoke/.venv/lib/site.py":           {Data: []byte("client.users_list()\n")},
		"tests/conformance/fixtures/x_test.py":    {Data: []byte("client.users_list()\n")},
		"tests/smoke/__pycache__/ignored_test.py": {Data: []byte("client.users_list()\n")},
	}
	got, err := capabilities.SDKTests(repo, []string{"auth.test", "chat.postMessage", "conversations.history", "users.list"})
	if err != nil {
		t.Fatalf("SDKTests: %v", err)
	}
	want := map[string][]string{
		"auth.test":             {"tests/smoke/a_test.py"},
		"chat.postMessage":      {"tests/e2e/b_test.py"},
		"conversations.history": {"tests/smoke/a_test.py"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SDKTests mismatch (-want +got):\n%s", diff)
	}
}

func TestClientVersions(t *testing.T) {
	t.Parallel()

	lock := `version = 1

[[package]]
name = "slack-sdk"
version = "3.41.0"

[[package]]
name = "pytest"
version = "9.0.2"

[[package]]
name = "aiohttp"
version = "3.12.0"
`
	got, err := capabilities.ClientVersions([]byte(lock), "slack-sdk", "slack-bolt", "aiohttp")
	if err != nil {
		t.Fatalf("ClientVersions: %v", err)
	}
	want := []capabilities.Client{
		{Name: "slack-sdk", Version: "3.41.0"},
		{Name: "slack-bolt"}, // not locked yet
		{Name: "aiohttp", Version: "3.12.0"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ClientVersions mismatch (-want +got):\n%s", diff)
	}
}

func TestEvidenceOrdering(t *testing.T) {
	t.Parallel()

	levels := []capabilities.Evidence{capabilities.EvidenceNone, capabilities.EvidenceLLM, capabilities.EvidenceDocs, capabilities.EvidenceRecorded}
	if !slices.IsSortedFunc(levels, func(a, b capabilities.Evidence) int { return capabilities.Rank(a) - capabilities.Rank(b) }) {
		t.Error("Rank must order none < llm < docs < recorded")
	}
}

func TestDeviationTests(t *testing.T) {
	t.Parallel()

	repo := fstest.MapFS{
		"tests/smoke/a_test.py":  {Data: []byte("@deviation(\"no-blocks\")\ndef test_a(): ...\n\n@deviation('other-gap')\ndef test_b(): ...\n")},
		"tests/smoke/b_test.py":  {Data: []byte("@deviation(\"no-blocks\")\ndef test_c(): ...\n# deviation(\"commented\") is not a marker\n")},
		"tests/smoke/.venv/x.py": {Data: []byte("@deviation(\"venv\")\n")},
	}
	got, err := capabilities.DeviationTests(repo)
	if err != nil {
		t.Fatalf("DeviationTests: %v", err)
	}
	want := map[string][]string{
		"no-blocks": {"tests/smoke/a_test.py", "tests/smoke/b_test.py"},
		"other-gap": {"tests/smoke/a_test.py"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DeviationTests mismatch (-want +got):\n%s", diff)
	}
}

// A red test tagged with a deviation id that is not in the manifest is a typo or a stale test.
func TestRepoDeviationTestsReferenceKnownDeviations(t *testing.T) {
	t.Parallel()

	m, err := capabilities.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tagged, err := capabilities.DeviationTests(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("DeviationTests: %v", err)
	}
	for id, files := range tagged {
		if _, ok := m.Deviations[id]; !ok {
			t.Errorf("@deviation(%q) in %v is not defined under [deviations.%s] in capabilities.toml", id, files, id)
		}
	}
	if len(tagged) == 0 {
		t.Error("no @deviation(...) tests found; the red-test scan is probably broken")
	}
}

func TestSuccessMethods(t *testing.T) {
	t.Parallel()

	fx := func(path string, ok bool) capabilities.NamedFixture {
		return capabilities.NamedFixture{Fixture: capabilities.Fixture{
			Request:  capabilities.FixtureRequest{Path: "/api/" + path},
			Response: capabilities.FixtureResponse{Body: map[string]any{"ok": ok}},
		}}
	}
	got := capabilities.SuccessMethods([]capabilities.NamedFixture{fx("a.ok", true), fx("a.ok", false), fx("b.err", false)})
	want := map[string]bool{"a.ok": true}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SuccessMethods mismatch (-want +got):\n%s", diff)
	}
}
