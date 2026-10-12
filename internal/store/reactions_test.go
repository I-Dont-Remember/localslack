package store

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReactionsRoundTrip(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	fixtureTx(t, s)
	in := []Reaction{
		{ChannelID: "C1", TS: "100.000001", Name: "eyes", UserID: "U1", Created: 5},
		{ChannelID: "C1", TS: "100.000001", Name: "+1", UserID: "U1", Created: 5},
		{ChannelID: "C1", TS: "100.000002", Name: "tada", UserID: "U1", Created: 6},
	}
	err := s.Write(t.Context(), func(tx *Tx) error {
		if err := tx.InsertMessage(t.Context(), Message{ChannelID: "C1", TS: "100.000002", UserID: "U1", Text: "two"}); err != nil {
			return err
		}
		for _, r := range in {
			if err := tx.InsertReaction(t.Context(), r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := s.ReactionsFor(t.Context(), "C1", []string{"100.000001", "100.000002", "100.000003"})
	if err != nil {
		t.Fatalf("ReactionsFor: %v", err)
	}
	want := map[string][]Reaction{"100.000001": in[:2], "100.000002": in[2:]}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ReactionsFor (-want +got):\n%s", diff)
	}

	err = s.Write(t.Context(), func(tx *Tx) error { return tx.InsertReaction(t.Context(), in[0]) })
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate insert err = %v, want ErrDuplicate", err)
	}

	err = s.Write(t.Context(), func(tx *Tx) error { return tx.DeleteReactions(t.Context(), "C1", "100.000001") })
	if err != nil {
		t.Fatalf("DeleteReactions: %v", err)
	}
	got, err = s.ReactionsFor(t.Context(), "C1", []string{"100.000001", "100.000002"})
	if err != nil || len(got) != 1 || len(got["100.000002"]) != 1 {
		t.Errorf("after delete = %v, %v; want only the other message's reactions", got, err)
	}
}
