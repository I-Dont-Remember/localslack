package capabilities_test

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"localslack/internal/capabilities"
)

func TestRunDocsWritesRenderedFile(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	written := map[string][]byte{}
	write := func(name string, data []byte) error { written[name] = data; return nil }
	if err := capabilities.Run([]string{"docs"}, fstest.MapFS{}, write, &out); err != nil {
		t.Fatalf("Run docs: %v", err)
	}
	got := string(written["docs/COMPATIBILITY.md"])
	if !strings.HasPrefix(got, "# Compatibility") || !strings.Contains(got, "`auth.test`") {
		t.Errorf("docs/COMPATIBILITY.md not written with rendered content: %.80q", got)
	}
}

func TestRunGapsListsShippedMethodsBelowRecorded(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := capabilities.Run([]string{"gaps"}, fstest.MapFS{}, nil, &out); err != nil {
		t.Fatalf("Run gaps: %v", err)
	}
	// With no fixtures at all every shipped method has no evidence.
	for _, want := range []string{"auth.test", "conversations.history", "none"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("gaps output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "conversations.leave") {
		t.Errorf("planned methods must not appear in gaps:\n%s", out.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"bogus"}} {
		if err := capabilities.Run(args, fstest.MapFS{}, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("Run(%v) err = %v, want usage error", args, err)
		}
	}
}
