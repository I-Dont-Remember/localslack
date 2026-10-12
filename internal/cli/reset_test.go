package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestResetCmdRun(t *testing.T) {
	t.Parallel()

	wantAbs, err := filepath.Abs("acme.json")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		cmd        ResetCmd
		status     int
		reply      string
		wantBody   map[string]any
		wantQuery  string
		wantOut    string
		wantErrSub string
	}{
		{
			name: "plain reset", status: 200, reply: `{"mode":"partial","seed":"s"}`,
			wantBody: map[string]any{}, wantOut: `{"mode":"partial","seed":"s"}`,
		},
		{
			name: "all", cmd: ResetCmd{All: true}, status: 200, reply: `{"mode":"all"}`,
			wantBody: map[string]any{}, wantQuery: "all=true", wantOut: `{"mode":"all"}`,
		},
		{
			name: "seed and scenario (path made absolute)", cmd: ResetCmd{Seed: "7", Scenario: "acme.json"}, status: 200, reply: `{"mode":"partial"}`,
			wantBody: map[string]any{"seed": "7", "scenario": wantAbs}, wantOut: `{"mode":"partial"}`,
		},
		{name: "endpoint missing", status: 404, reply: `{"error":{"code":"not_implemented","message":"x"}}`, wantBody: map[string]any{}, wantErrSub: "not supported by this server"},
		{name: "server error code", status: 409, reply: `{"error":{"code":"reset_in_progress","message":"reset already running"}}`, wantBody: map[string]any{}, wantErrSub: "reset_in_progress: reset already running"},
		{name: "non-json error", status: 502, reply: `bad gateway`, wantBody: map[string]any{}, wantErrSub: "HTTP 502"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotBody map[string]any
			var gotPath, gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.Method + " " + r.URL.Path
				gotQuery = r.URL.RawQuery
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.reply))
			}))
			t.Cleanup(srv.Close)
			tt.cmd.Addr = strings.TrimPrefix(srv.URL, "http://")

			var out bytes.Buffer
			err := tt.cmd.run(t.Context(), &out)

			if gotPath != "POST /_localslack/reset" {
				t.Errorf("request = %q, want POST /_localslack/reset", gotPath)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
			if diff := cmp.Diff(tt.wantBody, gotBody); diff != "" {
				t.Errorf("request body (-want +got):\n%s", diff)
			}
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil || strings.TrimSpace(out.String()) != tt.wantOut {
				t.Errorf("run = %q, %v; want %q", out.String(), err, tt.wantOut)
			}
		})
	}
}

func TestResetCmdUnreachable(t *testing.T) {
	t.Parallel()

	cmd := ResetCmd{Addr: "127.0.0.1:1"}
	err := cmd.run(t.Context(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "is it running") {
		t.Errorf("err = %v, want a hint that the server may not be running", err)
	}
}
