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
	"strings"
	"testing"

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

type fixtureResponse struct {
	Status int            `json:"status"`
	Body   map[string]any `json:"body"`
	Ignore []string       `json:"ignore"`
}

// fixture state "known_gap" documents real-Slack behavior we do not match yet:
// a mismatch is reported as a skip; if it starts matching the test fails so the
// fixture gets flipped to "passing".
type fixture struct {
	State    string          `json:"state"`
	Note     string          `json:"note"`
	Request  fixtureRequest  `json:"request"`
	Response fixtureResponse `json:"response"`
}

func (f fixture) newRequest(ctx context.Context, baseURL string) (*http.Request, error) {
	var body io.Reader
	if len(f.Request.Params) > 0 {
		form := url.Values{}
		for k, v := range f.Request.Params {
			form.Set(k, v)
		}
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, f.Request.Method, baseURL+f.Request.Path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if f.Request.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Request.Token)
	}
	return req, nil
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
			req, err := fix.newRequest(t.Context(), app.URL)
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

			var diffs []string
			if resp.StatusCode != fix.Response.Status {
				diffs = append(diffs, fmt.Sprintf("status: got %d, want %d", resp.StatusCode, fix.Response.Status))
			}
			var gotBody map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			diffs = append(diffs, diffBody(fix.Response.Body, gotBody, fix.Response.Ignore)...)

			report(t, fix, diffs)
		})
	}
}

func report(t *testing.T, fix fixture, diffs []string) {
	t.Helper()
	switch fix.State {
	case stateKnownGap:
		if len(diffs) == 0 {
			t.Error("known_gap fixture now matches; set \"state\": \"passing\"")
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
