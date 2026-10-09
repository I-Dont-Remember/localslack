package conformance_test

import "testing"

func TestGapVerdict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		want    []string
		got     []string
		wantMsg bool
	}{
		{"same set any order", []string{"$.a: missing", "$.b: extra"}, []string{"$.b: extra", "$.a: missing"}, false},
		{"fully fixed", []string{"$.a: missing"}, nil, true},
		{"new diff appears (partly right -> broken)", []string{"$.a: missing"}, []string{"$.a: missing", "$.c: wrong"}, true},
		{"one diff fixed", []string{"$.a: missing", "$.b: extra"}, []string{"$.a: missing"}, true},
		{"no gap_diffs recorded", nil, []string{"$.a: missing"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := gapVerdict(tt.want, tt.got)
			if (msg != "") != tt.wantMsg {
				t.Errorf("gapVerdict message = %q, want message=%v", msg, tt.wantMsg)
			}
		})
	}
}

func TestSubstitute(t *testing.T) {
	t.Parallel()

	got := substitute("/x/{{id}}?a={{id}}&b={{none}}", map[string]string{"id": "C1"})
	if want := "/x/C1?a=C1&b={{none}}"; got != want {
		t.Errorf("substitute = %q, want %q", got, want)
	}
}

func TestCapturePath(t *testing.T) {
	t.Parallel()

	body := map[string]any{"channel": map[string]any{"id": "C1"}, "n": 3.0}
	tests := []struct {
		path string
		want string
		ok   bool
	}{
		{"channel.id", "C1", true},
		{"n", "3", true},
		{"channel.nope", "", false},
		{"nope", "", false},
	}
	for _, tt := range tests {
		got, ok := capturePath(body, tt.path)
		if got != tt.want || ok != tt.ok {
			t.Errorf("capturePath(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.ok)
		}
	}
}
