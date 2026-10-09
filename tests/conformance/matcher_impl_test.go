package conformance_test

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	tsPattern = regexp.MustCompile(`^\d+\.\d{6}$`)
	idPattern = regexp.MustCompile(`^<id:([A-Z])>$`)
	idBody    = regexp.MustCompile(`^[A-Z0-9]+$`)
)

// diffBody compares got against want and returns human-readable differences (empty = match).
// String values in want may be matchers: <string>, <ts>, <id:X>, <any>.
// Keys named in ignore are skipped at any depth.
func diffBody(want, got any, ignore []string) []string {
	var diffs []string
	walk("$", want, got, ignore, &diffs)
	return diffs
}

func walk(path string, want, got any, ignore []string, diffs *[]string) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: got %v, want object", path, got))
			return
		}
		walkMap(path, w, g, ignore, diffs)
	case []any:
		g, ok := got.([]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: got %v, want array", path, got))
			return
		}
		if len(g) != len(w) {
			*diffs = append(*diffs, fmt.Sprintf("%s: got %d elements, want %d", path, len(g), len(w)))
			return
		}
		for i := range w {
			walk(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], ignore, diffs)
		}
	case string:
		if !matchString(w, got) {
			*diffs = append(*diffs, fmt.Sprintf("%s: got %s, want %s", path, quote(got), w))
		}
	default:
		if !reflect.DeepEqual(want, got) {
			*diffs = append(*diffs, fmt.Sprintf("%s: got %v, want %v", path, got, want))
		}
	}
}

func walkMap(path string, want, got map[string]any, ignore []string, diffs *[]string) {
	keys := make([]string, 0, len(want)+len(got))
	for k := range want {
		keys = append(keys, k)
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if slices.Contains(ignore, k) {
			continue
		}
		p := path + "." + k
		w, inWant := want[k]
		g, inGot := got[k]
		switch {
		case !inGot:
			*diffs = append(*diffs, p+": missing")
		case !inWant:
			*diffs = append(*diffs, p+": unexpected key")
		default:
			walk(p, w, g, ignore, diffs)
		}
	}
}

func matchString(want string, got any) bool {
	if want == "<any>" {
		return true
	}
	s, isStr := got.(string)
	if !isStr {
		return false
	}
	switch want {
	case "<string>":
		return s != ""
	case "<ts>":
		return tsPattern.MatchString(s)
	}
	if m := idPattern.FindStringSubmatch(want); m != nil {
		return len(s) >= 9 && strings.HasPrefix(s, m[1]) && idBody.MatchString(s)
	}
	return s == want
}

func quote(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}
