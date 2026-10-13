package store

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBulkInsertsEveryKindAndReadsBack(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	msgs := []Message{
		{ChannelID: "C1", TS: "1.000001", UserID: "U1", Text: "root", ReplyCount: 1, LatestReply: "2.000001"},
		{
			ChannelID: "C1", TS: "2.000001", UserID: "U2", Text: "reply", ThreadTS: "1.000001",
			BotID: "B1", Subtype: "x", Blocks: json.RawMessage(`[{"type":"divider"}]`), Edited: &Edit{User: "U2", TS: "3.000001"},
		},
	}
	err := s.Write(ctx, func(tx *Tx) error {
		b, err := tx.Bulk(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = b.Close() }()
		steps := []error{
			tx.InsertTeam(ctx, Team{ID: "T1", Name: "n", Domain: "d"}),
			b.User(ctx, User{ID: "U1", TeamID: "T1", Name: "ann"}),
			b.User(ctx, User{ID: "U2", TeamID: "T1", Name: "bot", IsBot: true, BotID: "B1"}),
			b.Conversation(ctx, Conversation{ID: "C1", TeamID: "T1", Name: "general", Kind: KindChannel, Creator: "U1", Created: 9, IsPrivate: true}),
			b.Member(ctx, "C1", "U1"),
			b.Member(ctx, "C1", "U2"),
			b.Message(ctx, msgs[0]),
			b.Message(ctx, msgs[1]),
			b.Reaction(ctx, Reaction{ChannelID: "C1", TS: "1.000001", Name: "eyes", UserID: "U1", Created: 1}),
		}
		for _, e := range steps {
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := s.Messages(ctx, "C1")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if diff := cmp.Diff(msgs, got); diff != "" {
		t.Errorf("messages (-want +got):\n%s", diff)
	}
	c, err := s.Conversation(ctx, "C1")
	if err != nil || !c.IsPrivate || c.Created != 9 || c.Creator != "U1" {
		t.Errorf("conversation = %+v, %v", c, err)
	}
	if bot, err := s.BotUser(ctx); err != nil || bot.BotID != "B1" {
		t.Errorf("bot = %+v, %v", bot, err)
	}
	r, err := s.ReactionsFor(ctx, "C1", []string{"1.000001"})
	if err != nil || len(r["1.000001"]) != 1 {
		t.Errorf("reactions = %v, %v", r, err)
	}
}

func TestBulkDuplicateIsErrDuplicate(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	err := s.Write(ctx, func(tx *Tx) error {
		b, err := tx.Bulk(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = b.Close() }()
		if err := tx.InsertTeam(ctx, Team{ID: "T1", Name: "n", Domain: "d"}); err != nil {
			return err
		}
		if err := b.User(ctx, User{ID: "U1", TeamID: "T1", Name: "a"}); err != nil {
			return err
		}
		return b.User(ctx, User{ID: "U1", TeamID: "T1", Name: "b"})
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate user err = %v, want ErrDuplicate", err)
	}
}

func TestTxTeamHelpers(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	ctx := t.Context()
	err := s.Write(ctx, func(tx *Tx) error {
		if empty, err := tx.IsEmpty(ctx); err != nil || !empty {
			t.Errorf("IsEmpty before = %v, %v; want true", empty, err)
		}
		if err := tx.InsertTeam(ctx, Team{ID: "T1", Name: "old", Domain: "old"}); err != nil {
			return err
		}
		if empty, err := tx.IsEmpty(ctx); err != nil || empty {
			t.Errorf("IsEmpty after = %v, %v; want false", empty, err)
		}
		return tx.RenameTeam(ctx, "New Name", "new")
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := s.Team(ctx)
	if want := (Team{ID: "T1", Name: "New Name", Domain: "new"}); err != nil || got != want {
		t.Errorf("team = %+v, %v; want %+v", got, err, want)
	}
}
