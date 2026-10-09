package capabilities_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"localslack/internal/api"
	"localslack/internal/capabilities"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		toml    string
		wantErr string // substring; "" = parses
	}{
		{"minimal", `[methods."a.b"]` + "\nstatus = \"planned\"\n", ""},
		{"unknown field is a typo, not ignored", `[methods."a.b"]` + "\nstatus = \"planned\"\nstatuz = 1\n", "statuz"},
		{"not toml", "[[[", "toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := capabilities.Parse([]byte(tt.toml))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Parse err = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Parse err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	repo := fstest.MapFS{"agent-docs/plans/real.md": {Data: []byte("plan")}}
	tests := []struct {
		name string
		toml string
		want []string // substrings expected among problems; nil = no problems
	}{
		{"valid planned method", `[methods."a.b"]` + "\nstatus = \"planned\"\n", nil},
		{"missing status", `[methods."a.b"]` + "\ntier = 2\n", []string{`method "a.b"`, "status"}},
		{"bad status", `[methods."a.b"]` + "\nstatus = \"done\"\n", []string{`"done"`}},
		{"bad tier", `[methods."a.b"]` + "\nstatus = \"planned\"\ntier = 9\n", []string{"tier"}},
		{"in_progress needs a plan", `[methods."a.b"]` + "\nstatus = \"in_progress\"\n", []string{"plan"}},
		{"in_progress plan must exist", `[methods."a.b"]` + "\nstatus = \"in_progress\"\nplan = \"agent-docs/plans/nope.md\"\n", []string{"nope.md"}},
		{"in_progress with real plan", `[methods."a.b"]` + "\nstatus = \"in_progress\"\nplan = \"agent-docs/plans/real.md\"\n", nil},
		{"unknown deviation reference", `[methods."a.b"]` + "\nstatus = \"planned\"\ndeviations = [\"ghost\"]\n", []string{`"ghost"`}},
		{
			"deviation needs severity and summary",
			`[deviations.d]` + "\nsummary = \"\"\nseverity = \"huge\"\n",
			[]string{"summary", "severity"},
		},
		{
			"implemented behavior needs evidence",
			`[behaviors.x]` + "\nstatus = \"implemented\"\nsummary = \"s\"\n",
			[]string{"behavior \"x\"", "evidence"},
		},
		{
			"docs evidence needs a citation",
			`[behaviors.x]` + "\nstatus = \"implemented\"\nsummary = \"s\"\nevidence = \"docs\"\n",
			[]string{"ref"},
		},
		{
			"behavior needs a summary",
			`[behaviors.x]` + "\nstatus = \"planned\"\n",
			[]string{"summary"},
		},
		{
			"valid implemented behavior",
			`[behaviors.x]` + "\nstatus = \"implemented\"\nsummary = \"s\"\nevidence = \"docs\"\nref = \"slack_sdk/web/base_client.py\"\n",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := capabilities.Parse([]byte(tt.toml))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := capabilities.Validate(m, repo)
			if tt.want == nil && len(got) != 0 {
				t.Errorf("Validate = %q, want no problems", got)
			}
			all := strings.Join(got, "\n")
			for _, w := range tt.want {
				if !strings.Contains(all, w) {
					t.Errorf("problems %q missing %q", got, w)
				}
			}
			if tt.want != nil && len(got) == 0 {
				t.Error("Validate found no problems, want some")
			}
		})
	}
}

func TestEmbeddedManifestIsValid(t *testing.T) {
	t.Parallel()

	m, err := capabilities.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if problems := capabilities.Validate(m, os.DirFS("../..")); len(problems) != 0 {
		t.Errorf("manifest problems:\n  %s", strings.Join(problems, "\n  "))
	}
}

// The manifest and the code must agree on which methods exist.
func TestRegistryParity(t *testing.T) {
	t.Parallel()

	m, err := capabilities.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	live := api.MethodNames()
	for name, meth := range m.Methods {
		shipped := meth.Status == capabilities.StatusImplemented || meth.Status == capabilities.StatusPartial
		inCode := slices.Contains(live, name)
		switch {
		case shipped && !inCode:
			t.Errorf("manifest says %s is %s but api.registry() has no such method", name, meth.Status)
		case !shipped && inCode:
			t.Errorf("api.registry() serves %s but the manifest says %s; set status implemented/partial", name, meth.Status)
		}
	}
	for _, name := range live {
		if _, ok := m.Methods[name]; !ok {
			t.Errorf("api.registry() serves %s but it is missing from internal/capabilities/capabilities.toml", name)
		}
	}
}
