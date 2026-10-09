package ids_test

import (
	"regexp"
	"testing"

	"localslack/internal/ids"
)

func TestGenerator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		gen    func(*ids.Generator) string
		prefix byte
	}{
		{"team", (*ids.Generator).Team, 'T'},
		{"user", (*ids.Generator).User, 'U'},
		{"channel", (*ids.Generator).Channel, 'C'},
		{"dm", (*ids.Generator).DM, 'D'},
	}
	shape := regexp.MustCompile(`^[A-Z][A-Z0-9]{9,10}$`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := ids.NewGenerator("seed")
			got := tt.gen(g)
			if !shape.MatchString(got) {
				t.Errorf("id %q does not match %v", got, shape)
			}
			if got[0] != tt.prefix {
				t.Errorf("id %q prefix = %c, want %c", got, got[0], tt.prefix)
			}
		})
	}
}

func TestGeneratorDeterminism(t *testing.T) {
	t.Parallel()

	seq := func(seed string) []string {
		g := ids.NewGenerator(seed)
		return []string{g.User(), g.User(), g.Channel(), g.DM()}
	}
	tests := []struct {
		name       string
		a, b       string
		wantEquals bool
	}{
		{"same seed", "1", "1", true},
		{"different seed", "1", "2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, b := seq(tt.a), seq(tt.b)
			equal := true
			for i := range a {
				equal = equal && a[i] == b[i]
			}
			if equal != tt.wantEquals {
				t.Errorf("seq(%q)==seq(%q) is %v, want %v\n%v\n%v", tt.a, tt.b, equal, tt.wantEquals, a, b)
			}
		})
	}
}

func TestGeneratorUnique(t *testing.T) {
	t.Parallel()

	g := ids.NewGenerator("u")
	seen := map[string]bool{}
	for range 1000 {
		id := g.User()
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}
