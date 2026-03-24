package conformance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"localslack/internal/server"
)

type fixtureRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type fixtureResponse struct {
	Status int            `json:"status"`
	Body   map[string]any `json:"body"`
}

type fixture struct {
	Request  fixtureRequest  `json:"request"`
	Response fixtureResponse `json:"response"`
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

	srv := httptest.NewServer(server.NewRouter())
	t.Cleanup(srv.Close)

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

			req, err := http.NewRequestWithContext(t.Context(), fix.Request.Method, srv.URL+fix.Request.Path, nil)
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

			if resp.StatusCode != fix.Response.Status {
				t.Errorf("status = %d, want %d", resp.StatusCode, fix.Response.Status)
			}

			var gotBody map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			wantJSON, _ := json.Marshal(fix.Response.Body)
			gotJSON, _ := json.Marshal(gotBody)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("body = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
