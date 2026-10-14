package service_test

import (
	"path/filepath"
	"testing"

	"localslack/internal/scenario"
	"localslack/internal/service"
)

// TestExampleScenariosLoadAndPopulate keeps examples/scenarios from rotting:
// every file loads, populates, and (when it carries data) reads back unchanged.
func TestExampleScenariosLoadAndPopulate(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("../../examples/scenarios/*")
	if err != nil || len(files) < 3 {
		t.Fatalf("example scenarios = %v, %v; want at least acme, scale, standup", files, err)
	}
	for _, file := range files {
		if scenario.FormatOf(file) == "" {
			continue // README and friends
		}
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			fx, err := scenario.LoadFile(file)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			svc, _ := newSvc(t)

			if _, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "examples", Scenario: &fx}); err != nil {
				t.Fatalf("Reset: %v", err)
			}

			if fx.Generate != nil {
				return
			}
			got, err := svc.Export(t.Context())
			if err != nil {
				t.Fatalf("Export: %v", err)
			}
			// Ids a file leaves out are drawn at reset time, so only fully pinned files compare by id.
			if fx.Users[0].ID == "" {
				if len(got.Users) != len(fx.Users) || len(got.Messages) != len(fx.Messages) {
					t.Errorf("exported %d users / %d messages, want %d / %d", len(got.Users), len(got.Messages), len(fx.Users), len(fx.Messages))
				}
				return
			}
			d := scenario.Diff(fx, got)
			if n := len(d.Users.Added) + len(d.Users.Removed) + len(d.Users.Changed) +
				len(d.Conversations.Added) + len(d.Conversations.Removed) + len(d.Conversations.Changed) +
				len(d.Members.Added) + len(d.Members.Removed) +
				len(d.Messages.Added) + len(d.Messages.Removed) + len(d.Messages.Changed) +
				len(d.Reactions.Added) + len(d.Reactions.Removed); n != 0 {
				t.Errorf("populated state differs from the file: %+v", d)
			}
		})
	}
}
