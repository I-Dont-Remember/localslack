package service_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestAddReaction(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	human, bot := usersOf(t, svc)
	ch := generalID(t, svc, human)
	root, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: "root"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	reply, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, ThreadTS: root.TS, Content: service.Content{Text: "reply"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	var c collector
	svc.Subscribe(c.handle)
	add := func(u service.User, ts, name string) error { return svc.AddReaction(t.Context(), u, ch, ts, name) }

	steps := []struct {
		name    string
		actor   service.User
		ts      string
		emoji   string
		wantErr error
	}{
		{"first", human, root.TS, "eyes", nil},
		{"second user same emoji", bot, root.TS, "eyes", nil},
		{"other emoji", bot, root.TS, "+1", nil},
		{"skin tone", human, root.TS, "wave::skin-tone-3", nil},
		{"on a reply", human, reply.TS, "tada", nil},
		{"duplicate", human, root.TS, "eyes", service.ErrAlreadyReacted},
		{"empty name", human, root.TS, "", service.ErrInvalidReactionName},
		{"colons", human, root.TS, ":eyes:", service.ErrInvalidReactionName},
		{"uppercase", human, root.TS, "Eyes", service.ErrInvalidReactionName},
		{"spaces", human, root.TS, "not a name", service.ErrInvalidReactionName},
		{"no message", human, "1.000001", "eyes", service.ErrMessageNotFound},
	}
	for _, s := range steps {
		if err := add(s.actor, s.ts, s.emoji); !errors.Is(err, s.wantErr) {
			t.Errorf("%s: err = %v, want %v", s.name, err, s.wantErr)
		}
	}
	if err := svc.AddReaction(t.Context(), human, "CNOPE", root.TS, "eyes"); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("unknown channel: err = %v, want ErrChannelNotFound", err)
	}

	hist, err := svc.History(t.Context(), human, ch, service.PageOpts{Limit: 1000, Oldest: root.TS, Inclusive: true})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	wantRoot := []service.Reaction{
		{Name: "eyes", Users: []string{human.ID, bot.ID}, Count: 2},
		{Name: "+1", Users: []string{bot.ID}, Count: 1},
		{Name: "wave::skin-tone-3", Users: []string{human.ID}, Count: 1},
	}
	if len(hist.Items) != 1 {
		t.Fatalf("history = %+v, want the root only", hist.Items)
	}
	if diff := cmp.Diff(wantRoot, hist.Items[0].Reactions); diff != "" {
		t.Errorf("history reactions (-want +got):\n%s", diff)
	}
	thread, err := svc.Replies(t.Context(), human, ch, root.TS, service.PageOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("Replies: %v", err)
	}
	if diff := cmp.Diff(wantRoot, thread.Items[0].Reactions); diff != "" {
		t.Errorf("thread root reactions (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]service.Reaction{{Name: "tada", Users: []string{human.ID}, Count: 1}}, thread.Items[1].Reactions); diff != "" {
		t.Errorf("reply reactions (-want +got):\n%s", diff)
	}

	var added []service.ReactionAdded
	for _, e := range c.got() {
		if ev, ok := e.(service.ReactionAdded); ok {
			added = append(added, ev)
		}
	}
	if len(added) != 5 || added[0].User != human.ID || added[0].Reaction != "eyes" || added[0].Msg.UserID != root.UserID || added[0].Msg.TS != root.TS {
		t.Errorf("ReactionAdded events = %+v, want 5 starting with human eyes on the root", added)
	}

	if err := svc.DeleteMessage(t.Context(), human, ch, reply.TS); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	left, err := st.ReactionsFor(t.Context(), ch, []string{reply.TS})
	if err != nil || len(left) != 0 {
		t.Errorf("reactions after deleting the message = %v, %v; want none", left, err)
	}
}
