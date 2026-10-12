package store

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUpdateApp(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	a := App{ID: "A1", Name: "demo", Manifest: []byte(`{}`), BotEvents: []string{}, Scopes: []string{}, Created: 7}
	if err := s.Write(ctx, func(tx *Tx) error { return tx.InsertApp(ctx, a) }); err != nil {
		t.Fatalf("InsertApp: %v", err)
	}

	want := a
	want.Name, want.DeliverTo, want.Verified = "renamed", "http://localhost:4000", true
	want.BotEvents = []string{"app_mention"}
	changed := want
	changed.Created = 999 // created is immutable: UpdateApp must not touch it
	if err := s.Write(ctx, func(tx *Tx) error { return tx.UpdateApp(ctx, changed) }); err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}
	got, err := s.App(ctx, "A1")
	if err != nil {
		t.Fatalf("App: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("App after update (-want +got):\n%s", diff)
	}

	missing := s.Write(ctx, func(tx *Tx) error { return tx.UpdateApp(ctx, App{ID: "nope"}) })
	if !errors.Is(missing, ErrNotFound) {
		t.Errorf("UpdateApp(missing) err = %v, want ErrNotFound", missing)
	}
}
