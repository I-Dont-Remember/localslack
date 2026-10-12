package store

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAppsCRUD(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	a := App{
		ID: "A1", Name: "demo", Manifest: []byte(`{"display_information":{"name":"demo"}}`),
		SigningSecret: "sec", BotToken: "bt", AppToken: "at", VerificationToken: "vt",
		RequestURL: "http://127.0.0.1:1/x", BotEvents: []string{"message.channels"}, Scopes: []string{"chat:write"},
		SocketMode: true, Verified: true, BotUserID: "UB", BotID: "BB", Created: 5, DeliverTo: "http://localhost:3000",
	}
	b := App{ID: "A2", Name: "other", Manifest: []byte(`{}`), BotEvents: []string{}, Scopes: []string{}}

	err := s.Write(ctx, func(tx *Tx) error {
		if err := tx.InsertApp(ctx, b); err != nil {
			return err
		}
		return tx.InsertApp(ctx, a)
	})
	if err != nil {
		t.Fatalf("InsertApp: %v", err)
	}

	got, err := s.App(ctx, "A1")
	if err != nil {
		t.Fatalf("App: %v", err)
	}
	if diff := cmp.Diff(a, got); diff != "" {
		t.Errorf("App (-want +got):\n%s", diff)
	}
	list, err := s.Apps(ctx)
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if diff := cmp.Diff([]App{a, b}, list); diff != "" {
		t.Errorf("Apps ordered by id (-want +got):\n%s", diff)
	}

	dup := s.Write(ctx, func(tx *Tx) error { return tx.InsertApp(ctx, a) })
	if !errors.Is(dup, ErrDuplicate) {
		t.Errorf("duplicate insert err = %v, want ErrDuplicate", dup)
	}
	if err := s.Write(ctx, func(tx *Tx) error { return tx.DeleteApp(ctx, "A1") }); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if _, err := s.App(ctx, "A1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("App after delete err = %v, want ErrNotFound", err)
	}
	if err := s.Write(ctx, func(tx *Tx) error { return tx.DeleteApp(ctx, "A1") }); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete err = %v, want ErrNotFound", err)
	}
}
