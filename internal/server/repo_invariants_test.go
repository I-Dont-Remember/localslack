package server_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"localslack/internal/config"
)

// unrestrictedPackages have no depguard rule on purpose. A new internal/ package must either get a
// rule in .golangci.yml (files: ["**/internal/<pkg>/**"]) or be listed here with a reason.
func unrestrictedPackages() map[string]string {
	return map[string]string{
		"capabilities": "manifest + codegen; pure data, no layering to protect",
		"cli":          "composition root: wires server, config and capabilities",
		"config":       "plain struct + env parsing",
		"constants":    "leaf constants",
		"ids":          "leaf ID helpers",
		"seed":         "fixture data generator, used by service and storetest",
		"server":       "composition root: wires every layer",
		"store":        "bottom layer; others are denied from importing it, it imports nothing above",
		"storetest":    "test helpers for every layer",
		"xdg":          "leaf path helpers",
	}
}

func TestEveryInternalPackageHasADepguardRuleOrReason(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Linters struct {
			Settings struct {
				Depguard struct {
					Rules map[string]struct {
						Files []string `yaml:"files"`
					} `yaml:"rules"`
				} `yaml:"depguard"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	pkgGlob := regexp.MustCompile(`internal/([a-z0-9_]+)/\*\*`)
	covered := map[string]bool{}
	for _, rule := range cfg.Linters.Settings.Depguard.Rules {
		for _, f := range rule.Files {
			if m := pkgGlob.FindStringSubmatch(f); m != nil {
				covered[m[1]] = true
			}
		}
	}

	entries, err := os.ReadDir(filepath.Join("..", "..", "internal"))
	if err != nil {
		t.Fatal(err)
	}
	unrestricted := unrestrictedPackages()
	var got []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		_, listed := unrestricted[name]
		switch {
		case covered[name] && listed:
			t.Errorf("internal/%s has a depguard rule AND is in unrestrictedPackages: drop it from the list", name)
		case !covered[name] && !listed:
			got = append(got, name)
		}
	}
	sort.Strings(got)
	if len(got) > 0 {
		t.Errorf("internal packages with no depguard rule and no unrestrictedPackages entry: %v (add a rule to .golangci.yml, or list with a reason in repo_invariants_test.go)", got)
	}
	for name := range unrestricted {
		if _, err := os.Stat(filepath.Join("..", "..", "internal", name)); err != nil {
			t.Errorf("unrestrictedPackages lists %q but internal/%s does not exist: remove it", name, name)
		}
	}
}

func TestEveryConfigFieldIsConsumed(t *testing.T) {
	t.Parallel()

	var src strings.Builder
	err := filepath.WalkDir(filepath.Join("..", "..", "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "config" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	typ := reflect.TypeFor[config.Config]()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if !regexp.MustCompile(`\.` + name + `\b`).MatchString(src.String()) {
			t.Errorf("config.Config.%s is never read outside internal/config: wire it up or delete it (a parsed-but-ignored flag silently does nothing)", name)
		}
	}
}
