package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// queryFixture: team T1; users U1 (human), U2 (human), UB (bot);
// channels C1 "general", C2 "old" (archived); DM D1 between U1 and U2.
func queryFixture(t *testing.T) *Store {
	t.Helper()
	s := openTemp(t)
	ctx := t.Context()
	err := s.Write(ctx, func(tx *Tx) error {
		steps := []error{
			tx.InsertTeam(ctx, Team{ID: "T1", Name: "Acme", Domain: "acme"}),
			tx.InsertUser(ctx, User{ID: "U1", TeamID: "T1", Name: "ann"}),
			tx.InsertUser(ctx, User{ID: "U2", TeamID: "T1", Name: "bob"}),
			tx.InsertUser(ctx, User{ID: "UB", TeamID: "T1", Name: "bot", IsBot: true}),
			tx.InsertConversation(ctx, Conversation{ID: "C1", TeamID: "T1", Name: "general", Kind: KindChannel}),
			tx.InsertConversation(ctx, Conversation{ID: "C2", TeamID: "T1", Name: "old", Kind: KindChannel, IsArchived: true}),
			tx.InsertConversation(ctx, Conversation{ID: "D1", TeamID: "T1", Kind: KindIM}),
			tx.AddMember(ctx, "D1", "U1"),
			tx.AddMember(ctx, "D1", "U2"),
		}
		return errors.Join(steps...)
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return s
}

func ids(cs []Conversation) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func TestConversationsPage(t *testing.T) {
	t.Parallel()

	s := queryFixture(t)
	tests := []struct {
		name string
		q    ConvQuery
		want []string
	}{
		{"channels only", ConvQuery{Kinds: []string{KindChannel}}, []string{"C1", "C2"}},
		{"exclude archived", ConvQuery{Kinds: []string{KindChannel}, ExcludeArchived: true}, []string{"C1"}},
		{"im visible to member", ConvQuery{Kinds: []string{KindIM}, MemberID: "U1"}, []string{"D1"}},
		{"im hidden from non-member", ConvQuery{Kinds: []string{KindIM}, MemberID: "UB"}, []string{}},
		{"mixed kinds", ConvQuery{Kinds: []string{KindChannel, KindIM}, MemberID: "U2"}, []string{"C1", "C2", "D1"}},
		{"after cursor", ConvQuery{Kinds: []string{KindChannel}, AfterID: "C1"}, []string{"C2"}},
		{"limit", ConvQuery{Kinds: []string{KindChannel}, Limit: 1}, []string{"C1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.ConversationsPage(t.Context(), tt.q)
			if err != nil {
				t.Fatalf("ConversationsPage: %v", err)
			}
			if diff := cmp.Diff(tt.want, ids(got)); diff != "" {
				t.Errorf("ids mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLookups(t *testing.T) {
	t.Parallel()

	s := queryFixture(t)
	ctx := t.Context()

	if got, err := s.User(ctx, "U2"); err != nil || got.Name != "bob" {
		t.Errorf("User(U2) = %+v, %v", got, err)
	}
	if _, err := s.User(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("User(nope) err = %v, want ErrNotFound", err)
	}
	if got, err := s.BotUser(ctx); err != nil || got.ID != "UB" {
		t.Errorf("BotUser = %+v, %v", got, err)
	}
	if got, err := s.Team(ctx); err != nil || got.ID != "T1" {
		t.Errorf("Team = %+v, %v", got, err)
	}
	if got, err := s.Conversation(ctx, "C1"); err != nil || got.Name != "general" {
		t.Errorf("Conversation(C1) = %+v, %v", got, err)
	}
	if _, err := s.Conversation(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Conversation(nope) err = %v, want ErrNotFound", err)
	}
}

func TestHistoryAndThread(t *testing.T) {
	t.Parallel()

	s := queryFixture(t)
	ctx := t.Context()
	err := s.Write(ctx, func(tx *Tx) error {
		var steps []error
		for i := 1; i <= 4; i++ {
			steps = append(steps, tx.InsertMessage(ctx, Message{ChannelID: "C1", TS: fmt.Sprintf("%d.000001", 100+i), UserID: "U1", Text: "m"}))
		}
		steps = append(steps,
			tx.InsertMessage(ctx, Message{ChannelID: "C1", TS: "110.000001", UserID: "U2", Text: "reply", ThreadTS: "102.000001"}),
			tx.BumpThread(ctx, "C1", "102.000001", "110.000001"),
		)
		return errors.Join(steps...)
	})
	if err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	tss := func(ms []Message) []string {
		out := make([]string, 0, len(ms))
		for _, m := range ms {
			out = append(out, m.TS)
		}
		return out
	}

	t.Run("history is top-level newest first", func(t *testing.T) {
		t.Parallel()
		got, err := s.HistoryPage(ctx, "C1", HistoryQuery{})
		if err != nil {
			t.Fatalf("HistoryPage: %v", err)
		}
		want := []string{"104.000001", "103.000001", "102.000001", "101.000001"}
		if diff := cmp.Diff(want, tss(got)); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
		for _, m := range got {
			if m.TS == "102.000001" && (m.ReplyCount != 1 || m.LatestReply != "110.000001") {
				t.Errorf("parent = %+v, want reply_count 1 latest 110.000001", m)
			}
		}
	})
	t.Run("history before cursor with limit", func(t *testing.T) {
		t.Parallel()
		got, err := s.HistoryPage(ctx, "C1", HistoryQuery{Before: "104.000001", Limit: 2})
		if err != nil {
			t.Fatalf("HistoryPage: %v", err)
		}
		if diff := cmp.Diff([]string{"103.000001", "102.000001"}, tss(got)); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
	})
	t.Run("range bounds exclusive then inclusive", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			inclusive bool
			want      []string
		}{
			{false, []string{"103.000001", "102.000001"}},
			{true, []string{"104.000001", "103.000001", "102.000001", "101.000001"}},
		} {
			hq := HistoryQuery{Oldest: "101.000001", Latest: "104.000001", Inclusive: tc.inclusive}
			got, err := s.HistoryPage(ctx, "C1", hq)
			if err != nil {
				t.Fatalf("HistoryPage: %v", err)
			}
			if diff := cmp.Diff(tc.want, tss(got)); diff != "" {
				t.Errorf("inclusive=%v (-want +got):\n%s", tc.inclusive, diff)
			}
		}
	})
	t.Run("members of", func(t *testing.T) {
		t.Parallel()
		got, err := s.MembersOf(ctx, []string{"D1", "C1"})
		if err != nil {
			t.Fatalf("MembersOf: %v", err)
		}
		if diff := cmp.Diff(map[string][]string{"D1": {"U1", "U2"}}, got); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
	})
	t.Run("thread is parent then replies", func(t *testing.T) {
		t.Parallel()
		got, err := s.ThreadPage(ctx, "C1", "102.000001", "", 0)
		if err != nil {
			t.Fatalf("ThreadPage: %v", err)
		}
		if diff := cmp.Diff([]string{"102.000001", "110.000001"}, tss(got)); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
	})
	t.Run("thread after cursor", func(t *testing.T) {
		t.Parallel()
		got, err := s.ThreadPage(ctx, "C1", "102.000001", "102.000001", 0)
		if err != nil {
			t.Fatalf("ThreadPage: %v", err)
		}
		if diff := cmp.Diff([]string{"110.000001"}, tss(got)); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
	})
}

func TestDMBetween(t *testing.T) {
	t.Parallel()

	s := queryFixture(t)
	tests := []struct {
		name   string
		kind   string
		users  []string
		wantID string
	}{
		{"existing pair", KindIM, []string{"U1", "U2"}, "D1"},
		{"order independent", KindIM, []string{"U2", "U1"}, "D1"},
		{"different pair", KindIM, []string{"U1", "UB"}, ""},
		{"self is not the pair", KindIM, []string{"U1"}, ""},
		{"wrong kind", KindMPIM, []string{"U1", "U2"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got string
			err := s.Write(t.Context(), func(tx *Tx) error {
				var err error
				got, err = tx.ConversationWithMembers(t.Context(), tt.kind, tt.users)
				return err
			})
			if err != nil {
				t.Fatalf("ConversationWithMembers: %v", err)
			}
			if got != tt.wantID {
				t.Errorf("got %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestAllMessages(t *testing.T) {
	t.Parallel()

	s := queryFixture(t)
	ctx := t.Context()
	want := []Message{
		{ChannelID: "C1", TS: "1.000001", UserID: "U1", Text: "a"},
		{ChannelID: "C1", TS: "3.000001", UserID: "U2", Text: "c"},
		{ChannelID: "D1", TS: "2.000001", UserID: "U1", Text: "b"},
	}
	err := s.Write(ctx, func(tx *Tx) error {
		// inserted out of order on purpose
		return errors.Join(
			tx.InsertMessage(ctx, want[2]), tx.InsertMessage(ctx, want[1]), tx.InsertMessage(ctx, want[0]),
		)
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.AllMessages(ctx)
	if err != nil {
		t.Fatalf("AllMessages: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("AllMessages mismatch (-want +got):\n%s", diff)
	}
}
