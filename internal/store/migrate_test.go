package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestMigrateAllowsOutOfOrder(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mig := func(table string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE " + table + " (id INTEGER);\n-- +goose Down\nDROP TABLE " + table + ";\n")}
	}
	// A branch ships v3 first; another branch's v2 lands later.
	if err := migrateFS(t.Context(), db, fstest.MapFS{"00001_a.sql": mig("a"), "00003_c.sql": mig("c")}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	all := fstest.MapFS{"00001_a.sql": mig("a"), "00002_b.sql": mig("b"), "00003_c.sql": mig("c")}
	if err := migrateFS(t.Context(), db, all); err != nil {
		t.Fatalf("second migrate (out of order): %v", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_master WHERE name = 'b'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("table b exists = %d, %v; want 1", n, err)
	}
}

func TestTestLoopSchema(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	err := s.Write(ctx, func(tx *Tx) error {
		if err := tx.InsertTeam(ctx, Team{ID: "T1", Name: "Acme", Domain: "acme"}); err != nil {
			return err
		}
		if err := tx.InsertUser(ctx, User{ID: "UB", TeamID: "T1", Name: "bot", IsBot: true, BotID: "BB"}); err != nil {
			return err
		}
		return tx.InsertConversation(ctx, Conversation{ID: "G1", TeamID: "T1", Name: "secret", Kind: KindChannel, IsPrivate: true})
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	u, err := s.User(ctx, "UB")
	if err != nil || u.BotID != "BB" {
		t.Errorf("User = %+v, %v; want BotID BB", u, err)
	}
	c, err := s.Conversation(ctx, "G1")
	if err != nil || !c.IsPrivate {
		t.Errorf("Conversation = %+v, %v; want IsPrivate", c, err)
	}
	var n int
	q := `SELECT count(*) FROM apps`
	if err := s.reader.QueryRowContext(ctx, q).Scan(&n); err != nil || n != 0 {
		t.Errorf("apps table: %d, %v; want empty table", n, err)
	}
}
