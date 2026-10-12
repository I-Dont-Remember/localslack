package seed_test

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"localslack/internal/seed"
	"localslack/internal/store"
)

func fixedNow() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

type dump struct {
	Users    []store.User
	Convs    []store.Conversation
	Messages map[string][]store.Message
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
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

func seeded(t *testing.T, seedStr string) (*store.Store, dump) {
	t.Helper()
	s := openStore(t)
	did, err := seed.Apply(t.Context(), s, seed.Options{Seed: seedStr, Now: fixedNow()})
	if err != nil || !did {
		t.Fatalf("Apply = %v, %v; want true, nil", did, err)
	}
	return s, read(t, s)
}

func read(t *testing.T, s *store.Store) dump {
	t.Helper()
	users, err := s.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	convs, err := s.Conversations(t.Context())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	d := dump{Users: users, Convs: convs, Messages: map[string][]store.Message{}}
	for _, c := range convs {
		msgs, err := s.Messages(t.Context(), c.ID)
		if err != nil {
			t.Fatalf("Messages: %v", err)
		}
		d.Messages[c.ID] = msgs
	}
	return d
}

func TestSameSeedSameData(t *testing.T) {
	t.Parallel()

	_, a := seeded(t, "42")
	_, b := seeded(t, "42")
	if diff := cmp.Diff(a, b); diff != "" {
		t.Errorf("same seed produced different data (-a +b):\n%s", diff)
	}
}

func TestDifferentSeedDifferentData(t *testing.T) {
	t.Parallel()

	_, a := seeded(t, "1")
	_, b := seeded(t, "2")
	if cmp.Equal(a, b) {
		t.Error("different seeds produced identical data")
	}
}

func TestShapeAcrossSeeds(t *testing.T) {
	t.Parallel()

	for i := range 40 {
		name := "seed-" + strconv.Itoa(i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, d := seeded(t, name)
			checkShape(t, d)
		})
	}
}

func checkShape(t *testing.T, d dump) {
	t.Helper()

	var humans, bots int
	for _, u := range d.Users {
		if u.IsBot {
			bots++
			if u.Name != "localslack-bot" {
				t.Errorf("bot name = %q, want localslack-bot", u.Name)
			}
			if len(u.BotID) != 11 || u.BotID[0] != 'B' {
				t.Errorf("bot_id = %q, want B + 10 chars", u.BotID)
			}
		} else {
			humans++
			if u.BotID != "" {
				t.Errorf("human %s has bot_id %q", u.ID, u.BotID)
			}
		}
	}
	if humans < 1 || humans > 10 || bots != 1 {
		t.Errorf("humans=%d bots=%d, want 1..10 humans and 1 bot", humans, bots)
	}
	if n := len(d.Convs); n < 2 || n > 50 {
		t.Errorf("conversations = %d, want 2..50", n)
	}

	var general string
	withMsgs := 0
	for _, c := range d.Convs {
		if c.Name == "general" {
			general = c.ID
		}
		if len(d.Messages[c.ID]) > 0 {
			withMsgs++
		}
	}
	if general == "" {
		t.Fatal("no #general channel")
	}
	if withMsgs != 1 || len(d.Messages[general]) == 0 {
		t.Errorf("channels with messages = %d (general has %d), want only #general", withMsgs, len(d.Messages[general]))
	}
	checkThread(t, d.Messages[general])
}

func checkThread(t *testing.T, msgs []store.Message) {
	t.Helper()

	if n := len(msgs); n < 3 || n > 25 {
		t.Errorf("#general messages = %d, want 3..25", n)
	}
	replies := map[string]int{}
	latest := map[string]string{}
	for _, m := range msgs {
		if m.ThreadTS != "" && m.ThreadTS != m.TS {
			replies[m.ThreadTS]++
			latest[m.ThreadTS] = m.TS
		}
	}
	if len(replies) != 1 {
		t.Fatalf("threads = %d, want 1", len(replies))
	}
	for _, m := range msgs {
		if want, ok := replies[m.TS]; ok {
			if m.ReplyCount != want || m.LatestReply != latest[m.TS] {
				t.Errorf("parent %s reply_count=%d latest=%q, want %d %q", m.TS, m.ReplyCount, m.LatestReply, want, latest[m.TS])
			}
		}
	}
}

func TestApplyOnlyWhenEmpty(t *testing.T) {
	t.Parallel()

	s, before := seeded(t, "7")
	did, err := seed.Apply(t.Context(), s, seed.Options{Seed: "8", Now: fixedNow()})
	if err != nil || did {
		t.Fatalf("second Apply = %v, %v; want false, nil", did, err)
	}
	if diff := cmp.Diff(before, read(t, s)); diff != "" {
		t.Errorf("second Apply changed data (-before +after):\n%s", diff)
	}
}

func TestPopulateKeepsGivenTeamAndSeededData(t *testing.T) {
	t.Parallel()

	_, want := seeded(t, "42")
	s := openStore(t)
	keep := store.Team{ID: "TKEEP", Name: "kept", Domain: "kept"}
	err := s.Write(t.Context(), func(tx *store.Tx) error {
		if err := tx.InsertTeam(t.Context(), keep); err != nil {
			return err
		}
		return seed.Populate(t.Context(), tx, seed.Options{Seed: "42", Now: fixedNow()}, &keep)
	})
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}

	got := read(t, s)

	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(store.User{}, "TeamID"), cmpopts.IgnoreFields(store.Conversation{}, "TeamID")); diff != "" {
		t.Errorf("ids and content must match a fresh seed (-want +got):\n%s", diff)
	}
	for _, u := range got.Users {
		if u.TeamID != "TKEEP" {
			t.Errorf("user %s team = %q, want TKEEP", u.ID, u.TeamID)
		}
	}
	if teams, err := s.Team(t.Context()); err != nil || teams.ID != "TKEEP" {
		t.Errorf("team = %+v, %v; want TKEEP only", teams, err)
	}
}
