package store

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func populate(t *testing.T, s *Store) {
	t.Helper()
	err := s.Write(t.Context(), func(tx *Tx) error {
		ctx := t.Context()
		steps := []error{
			tx.InsertTeam(ctx, Team{ID: "T1", Name: "n", Domain: "d"}),
			tx.InsertUser(ctx, User{ID: "U1", TeamID: "T1", Name: "ada"}),
			tx.InsertConversation(ctx, Conversation{ID: "C1", TeamID: "T1", Name: "general", Kind: KindChannel}),
			tx.AddMember(ctx, "C1", "U1"),
			tx.InsertMessage(ctx, Message{ChannelID: "C1", TS: "1.000001", UserID: "U1", Text: "hi"}),
			tx.InsertApp(ctx, App{ID: "A1", BotUserID: "UB"}),
		}
		for _, err := range steps {
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("populate: %v", err)
	}
}

func tableCounts(t *testing.T, s *Store) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tbl := range []string{"teams", "users", "conversations", "members", "messages", "apps"} {
		var n int
		if err := s.reader.QueryRowContext(t.Context(), "SELECT count(*) FROM "+tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		out[tbl] = n
	}
	return out
}

func TestWipeWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keep []string
		want map[string]int
	}{
		{
			"keep teams and apps",
			[]string{"teams", "apps"},
			map[string]int{"teams": 1, "users": 0, "conversations": 0, "members": 0, "messages": 0, "apps": 1},
		},
		{
			"keep teams only",
			[]string{"teams"},
			map[string]int{"teams": 1, "users": 0, "conversations": 0, "members": 0, "messages": 0, "apps": 0},
		},
		{
			"keep nothing", nil,
			map[string]int{"teams": 0, "users": 0, "conversations": 0, "members": 0, "messages": 0, "apps": 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := openTemp(t)
			populate(t, s)

			err := s.Write(t.Context(), func(tx *Tx) error { return tx.WipeWorkspace(t.Context(), tt.keep...) })
			if err != nil {
				t.Fatalf("WipeWorkspace: %v", err)
			}
			if diff := cmp.Diff(tt.want, tableCounts(t, s)); diff != "" {
				t.Errorf("row counts (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWipeWorkspaceKeepsMigrationHistory(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	populate(t, s)

	if err := s.Write(t.Context(), func(tx *Tx) error { return tx.WipeWorkspace(t.Context()) }); err != nil {
		t.Fatalf("WipeWorkspace: %v", err)
	}

	if err := migrate(t.Context(), s.writer); err != nil {
		t.Errorf("re-running migrations after wipe: %v", err)
	}
}

func TestWipeWorkspaceRollsBackWithTx(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	populate(t, s)

	_ = s.Write(t.Context(), func(tx *Tx) error {
		if err := tx.WipeWorkspace(t.Context()); err != nil {
			return err
		}
		return context.Canceled // force rollback
	})

	if got := tableCounts(t, s)["messages"]; got != 1 {
		t.Errorf("messages after rolled-back wipe = %d, want 1", got)
	}
}

func TestTxReadsForReset(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	populate(t, s)

	var team Team
	var apps []App
	var maxTS string
	var counts Counts
	err := s.Write(t.Context(), func(tx *Tx) (err error) {
		ctx := t.Context()
		if team, err = tx.Team(ctx); err != nil {
			return err
		}
		if apps, err = tx.Apps(ctx); err != nil {
			return err
		}
		if maxTS, err = tx.MaxTS(ctx); err != nil {
			return err
		}
		counts, err = tx.Counts(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("tx reads: %v", err)
	}
	if team.ID != "T1" || len(apps) != 1 || apps[0].ID != "A1" || maxTS != "1.000001" {
		t.Errorf("team %+v apps %+v maxTS %q", team, apps, maxTS)
	}
	if want := (Counts{Users: 1, Conversations: 1, Messages: 1}); counts != want {
		t.Errorf("counts = %+v, want %+v", counts, want)
	}
}
