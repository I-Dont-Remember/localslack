package conformance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

const (
	statePassing  = "passing"
	stateKnownGap = "known_gap"
)

type fixtureRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Token  string            `json:"token"`
	Params map[string]string `json:"params"`
}

// setupStep is a request run before the fixture's own; Capture maps a name to a
// dotted path in the response body, usable as {{name}} in later paths and params.
type setupStep struct {
	fixtureRequest
	Capture map[string]string `json:"capture"`
}

type fixtureResponse struct {
	Status int            `json:"status"`
	Body   map[string]any `json:"body"`
	Ignore []string       `json:"ignore"`
}

// fixture state "known_gap" documents real-Slack behavior we do not match yet.
// gap_diffs records the exact set of differences; the test fails if that set
// changes in either direction, so progress and regressions are both noticed.
type fixture struct {
	State    string          `json:"state"`
	Note     string          `json:"note"`
	GapDiffs []string        `json:"gap_diffs"`
	Setup    []setupStep     `json:"setup"`
	Request  fixtureRequest  `json:"request"`
	Response fixtureResponse `json:"response"`
}

func (r fixtureRequest) newRequest(ctx context.Context, baseURL string, caps map[string]string) (*http.Request, error) {
	var body io.Reader
	if len(r.Params) > 0 {
		form := url.Values{}
		for k, v := range r.Params {
			form.Set(k, substitute(v, caps))
		}
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, baseURL+substitute(r.Path, caps), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", constants.ContentTypeForm)
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	return req, nil
}

func substitute(s string, caps map[string]string) string {
	for k, v := range caps {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

// capturePath reads a dotted path out of a decoded JSON body as a string.
func capturePath(body map[string]any, path string) (string, bool) {
	var cur any = body
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		if cur, ok = m[part]; !ok {
			return "", false
		}
	}
	switch v := cur.(type) {
	case string:
		return v, true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	}
	return "", false
}

func send(t *testing.T, baseURL string, r fixtureRequest, caps map[string]string) (int, map[string]any) {
	t.Helper()
	req, err := r.newRequest(t.Context(), baseURL, caps)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return resp.StatusCode, body
}

func TestConformance(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("fixtures/*.json")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixture files found")
	}

	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fix fixture
			if err := json.Unmarshal(data, &fix); err != nil {
				t.Fatalf("parse fixture: %v", err)
			}

			app := storetest.NewApp(t) // one isolated app per fixture
			caps := map[string]string{}
			for i, step := range fix.Setup {
				_, body := send(t, app.URL, step.fixtureRequest, caps)
				for name, p := range step.Capture {
					v, ok := capturePath(body, p)
					if !ok {
						t.Fatalf("setup[%d]: capture %q: no %q in %v", i, name, p, body)
					}
					caps[name] = v
				}
			}

			status, gotBody := send(t, app.URL, fix.Request, caps)
			var diffs []string
			if status != fix.Response.Status {
				diffs = append(diffs, fmt.Sprintf("status: got %d, want %d", status, fix.Response.Status))
			}
			diffs = append(diffs, diffBody(fix.Response.Body, gotBody, fix.Response.Ignore)...)

			report(t, fix, diffs)
		})
	}
}

// gapVerdict returns "" when the observed diffs equal the recorded gap_diffs
// (as sets), else a message describing the change.
func gapVerdict(recorded, observed []string) string {
	a, b := slices.Clone(recorded), slices.Clone(observed)
	slices.Sort(a)
	slices.Sort(b)
	if slices.Equal(a, b) {
		return ""
	}
	return fmt.Sprintf("known_gap diffs changed.\nrecorded gap_diffs: %q\nobserved:           %q\n"+
		"If this is progress, update gap_diffs (or set \"state\": \"passing\" when empty); if a regression, fix it.",
		a, b)
}

func report(t *testing.T, fix fixture, diffs []string) {
	t.Helper()
	switch fix.State {
	case stateKnownGap:
		if msg := gapVerdict(fix.GapDiffs, diffs); msg != "" {
			t.Error(msg)
			return
		}
		t.Skipf("KNOWN GAP (%s):\n  %s", fix.Note, strings.Join(diffs, "\n  "))
	case "", statePassing:
		for _, d := range diffs {
			t.Error(d)
		}
	default:
		t.Errorf("unknown fixture state %q", fix.State)
	}
}
