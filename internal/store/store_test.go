package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "sub", "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestPragmasOnBothPools(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	tests := []struct {
		pragma string
		want   string
	}{
		{"foreign_keys", "1"},
		{"busy_timeout", "5000"},
		{"journal_mode", "wal"},
	}
	for _, pool := range []struct {
		name string
		scan func(q string) (string, error)
	}{
		{"writer", func(q string) (string, error) {
			var v string
			err := s.writer.QueryRowContext(t.Context(), q).Scan(&v)
			return v, err
		}},
		{"reader", func(q string) (string, error) {
			var v string
			err := s.reader.QueryRowContext(t.Context(), q).Scan(&v)
			return v, err
		}},
	} {
		for _, tt := range tests {
			t.Run(pool.name+"/"+tt.pragma, func(t *testing.T) {
				t.Parallel()
				got, err := pool.scan("PRAGMA " + tt.pragma)
				if err != nil {
					t.Fatalf("pragma: %v", err)
				}
				if got != tt.want {
					t.Errorf("PRAGMA %s = %q, want %q", tt.pragma, got, tt.want)
				}
			})
		}
	}
}

func fixtureTx(t *testing.T, s *Store) {
	t.Helper()
	err := s.Write(t.Context(), func(tx *Tx) error {
		if err := tx.InsertTeam(t.Context(), Team{ID: "T1", Name: "Acme", Domain: "acme"}); err != nil {
			return err
		}
		if err := tx.InsertUser(t.Context(), User{ID: "U1", TeamID: "T1", Name: "ann", RealName: "Ann"}); err != nil {
			return err
		}
		if err := tx.InsertConversation(t.Context(), Conversation{ID: "C1", TeamID: "T1", Name: "general", Kind: KindChannel, Creator: "U1", Created: 100}); err != nil {
			return err
		}
		if err := tx.AddMember(t.Context(), "C1", "U1"); err != nil {
			return err
		}
		return tx.InsertMessage(t.Context(), Message{ChannelID: "C1", TS: "100.000001", UserID: "U1", Text: "hi"})
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestRoundTripAndEmpty(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	empty, err := s.IsEmpty(t.Context())
	if err != nil || !empty {
		t.Fatalf("IsEmpty = %v, %v; want true, nil", empty, err)
	}
	fixtureTx(t, s)

	if empty, err = s.IsEmpty(t.Context()); err != nil || empty {
		t.Errorf("IsEmpty after insert = %v, %v; want false, nil", empty, err)
	}
	gotMsgs, err := s.Messages(t.Context(), "C1")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	wantMsgs := []Message{{ChannelID: "C1", TS: "100.000001", UserID: "U1", Text: "hi"}}
	if diff := cmp.Diff(wantMsgs, gotMsgs); diff != "" {
		t.Errorf("Messages mismatch (-want +got):\n%s", diff)
	}
	gotUsers, err := s.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	wantUsers := []User{{ID: "U1", TeamID: "T1", Name: "ann", RealName: "Ann"}}
	if diff := cmp.Diff(wantUsers, gotUsers); diff != "" {
		t.Errorf("Users mismatch (-want +got):\n%s", diff)
	}
	gotConvs, err := s.Conversations(t.Context())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	wantConvs := []Conversation{{ID: "C1", TeamID: "T1", Name: "general", Kind: KindChannel, Creator: "U1", Created: 100}}
	if diff := cmp.Diff(wantConvs, gotConvs); diff != "" {
		t.Errorf("Conversations mismatch (-want +got):\n%s", diff)
	}
	if got, err := s.MaxTS(t.Context()); err != nil || got != "100.000001" {
		t.Errorf("MaxTS = %q, %v; want 100.000001, nil", got, err)
	}
}

func TestDuplicateChannelNameIsErrDuplicate(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	fixtureTx(t, s)
	err := s.Write(t.Context(), func(tx *Tx) error {
		return tx.InsertConversation(t.Context(), Conversation{ID: "C2", TeamID: "T1", Name: "GENERAL", Kind: KindChannel, Creator: "U1"})
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestWriteRollsBackOnError(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	boom := errors.New("boom")
	err := s.Write(t.Context(), func(tx *Tx) error {
		if err := tx.InsertTeam(t.Context(), Team{ID: "T1", Name: "x", Domain: "x"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if empty, err := s.IsEmpty(t.Context()); err != nil || !empty {
		t.Errorf("IsEmpty = %v, %v; want true, nil (rolled back)", empty, err)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	err := s.Write(t.Context(), func(tx *Tx) error {
		return tx.InsertMessage(t.Context(), Message{ChannelID: "nope", TS: "1.000001", UserID: "U", Text: "x"})
	})
	if err == nil {
		t.Error("InsertMessage into missing channel succeeded, want FK error")
	}
}

func TestReadWhileWriteDoesNotBlock(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	fixtureTx(t, s)
	err := s.Write(t.Context(), func(tx *Tx) error {
		if err := tx.InsertMessage(t.Context(), Message{ChannelID: "C1", TS: "101.000001", UserID: "U1", Text: "uncommitted"}); err != nil {
			return err
		}
		// Reader must see the last committed state, not block, while the writer txn is open.
		msgs, err := s.Messages(t.Context(), "C1")
		if err != nil {
			return err
		}
		if len(msgs) != 1 {
			t.Errorf("reader saw %d messages mid-txn, want 1 (committed only)", len(msgs))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	msgs, err := s.Messages(t.Context(), "C1")
	if err != nil || len(msgs) != 2 {
		t.Errorf("after commit: %d msgs, err %v; want 2, nil", len(msgs), err)
	}
}

func TestAfterCommit(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	fixtureTx(t, s)
	boom := errors.New("boom")

	t.Run("runs after commit, seeing the write", func(t *testing.T) {
		t.Parallel()
		var seen bool
		err := s.Write(t.Context(), func(tx *Tx) error {
			if err := tx.InsertMessage(t.Context(), Message{ChannelID: "C1", TS: "5.000001", UserID: "U1"}); err != nil {
				return err
			}
			tx.AfterCommit(func() {
				_, err := s.Message(t.Context(), "C1", "5.000001")
				seen = err == nil
			})
			return nil
		})
		if err != nil || !seen {
			t.Errorf("Write err = %v, hook saw committed row = %v; want nil, true", err, seen)
		}
	})

	t.Run("skipped on rollback", func(t *testing.T) {
		t.Parallel()
		ran := false
		err := s.Write(t.Context(), func(tx *Tx) error {
			tx.AfterCommit(func() { ran = true })
			return boom
		})
		if !errors.Is(err, boom) || ran {
			t.Errorf("err = %v, hook ran = %v; want boom, false", err, ran)
		}
	})

	t.Run("hook order is commit order", func(t *testing.T) {
		t.Parallel()
		const n = 20
		var mu sync.Mutex
		var order []int
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = s.Write(t.Context(), func(tx *Tx) error {
					ts := fmt.Sprintf("7.%06d", i+1)
					if err := tx.InsertMessage(t.Context(), Message{ChannelID: "C1", TS: ts, UserID: "U1"}); err != nil {
						return err
					}
					tx.AfterCommit(func() { mu.Lock(); order = append(order, i); mu.Unlock() })
					return nil
				})
			}()
		}
		wg.Wait()
		rows, err := s.reader.QueryContext(t.Context(), `SELECT ts FROM messages WHERE ts LIKE '7.%' ORDER BY rowid`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer func() { _ = rows.Close() }()
		var want []int
		for rows.Next() {
			var ts string
			var i int
			if err := rows.Scan(&ts); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if _, err := fmt.Sscanf(ts, "7.%d", &i); err != nil {
				t.Fatalf("parse %q: %v", ts, err)
			}
			want = append(want, i-1)
		}
		if diff := cmp.Diff(want, order); diff != "" {
			t.Errorf("hook order vs commit order (-commit +hooks):\n%s", diff)
		}
	})
}
