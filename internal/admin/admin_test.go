package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWriteError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		code   string
		msg    string
		want   map[string]any
	}{
		{
			"not found", http.StatusNotFound, "app_not_found", "no such app",
			map[string]any{"error": map[string]any{"code": "app_not_found", "message": "no such app"}},
		},
		{
			"internal", http.StatusInternalServerError, "internal_error", "internal error",
			map[string]any{"error": map[string]any{"code": "internal_error", "message": "internal error"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			log := slog.New(slog.NewTextHandler(io.Discard, nil))

			writeError(t.Context(), log, w, tt.status, tt.code, tt.msg)

			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("body (-want +got):\n%s", diff)
			}
		})
	}
}
