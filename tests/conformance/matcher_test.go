package conformance_test

import (
	"reflect"
	"testing"
)

func TestDiffBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		want   any
		got    any
		ignore []string
		diffs  []string
	}{
		{name: "equal", want: map[string]any{"ok": true}, got: map[string]any{"ok": true}},
		{
			name: "value mismatch", want: map[string]any{"ok": true}, got: map[string]any{"ok": false},
			diffs: []string{"$.ok: got false, want true"},
		},
		{
			name: "missing key", want: map[string]any{"a": "x"}, got: map[string]any{},
			diffs: []string{"$.a: missing"},
		},
		{
			name: "extra key", want: map[string]any{}, got: map[string]any{"b": 1.0},
			diffs: []string{"$.b: unexpected key"},
		},
		{
			name: "ignored key both sides", want: map[string]any{"ok": true, "ts": "1"},
			got: map[string]any{"ok": true, "ts": "2"}, ignore: []string{"ts"},
		},
		{
			name: "ignored key absent in got", want: map[string]any{"ok": true, "ts": "1"},
			got: map[string]any{"ok": true}, ignore: []string{"ts"},
		},
		{name: "string matcher", want: map[string]any{"s": "<string>"}, got: map[string]any{"s": "hi"}},
		{
			name: "string matcher rejects empty", want: map[string]any{"s": "<string>"}, got: map[string]any{"s": ""},
			diffs: []string{`$.s: got "", want <string>`},
		},
		{name: "ts matcher", want: map[string]any{"ts": "<ts>"}, got: map[string]any{"ts": "1712345678.000100"}},
		{
			name: "ts matcher rejects", want: map[string]any{"ts": "<ts>"}, got: map[string]any{"ts": "nope"},
			diffs: []string{`$.ts: got "nope", want <ts>`},
		},
		{name: "id matcher", want: map[string]any{"id": "<id:C>"}, got: map[string]any{"id": "C01ABCDEF2"}},
		{
			name: "id matcher wrong prefix", want: map[string]any{"id": "<id:C>"}, got: map[string]any{"id": "U01ABCDEF2"},
			diffs: []string{`$.id: got "U01ABCDEF2", want <id:C>`},
		},
		{name: "any matcher", want: map[string]any{"v": "<any>"}, got: map[string]any{"v": []any{1.0}}},
		{
			name: "nested array", want: map[string]any{"l": []any{map[string]any{"id": "<id:U>"}}},
			got: map[string]any{"l": []any{map[string]any{"id": "U123456789"}}},
		},
		{
			name: "array length", want: []any{1.0}, got: []any{1.0, 2.0},
			diffs: []string{"$: got 2 elements, want 1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := diffBody(tt.want, tt.got, tt.ignore)
			if !reflect.DeepEqual(got, tt.diffs) && (len(got) != 0 || len(tt.diffs) != 0) {
				t.Errorf("diffs = %q, want %q", got, tt.diffs)
			}
		})
	}
}
