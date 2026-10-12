package service_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestPostEphemeral(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	ch := generalID(t, svc, human)
	var c collector
	svc.Subscribe(c.handle)
	before, err := svc.History(t.Context(), human, ch, service.PageOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	tests := []struct {
		name    string
		channel string
		user    string
		content service.Content
		wantErr error
	}{
		{"ok", ch, human.ID, service.Content{Text: "psst"}, nil},
		{"blocks only", ch, human.ID, service.Content{Blocks: []byte(`[{"type":"divider"}]`)}, nil},
		{"user not in channel", ch, "UNOPE", service.Content{Text: "x"}, service.ErrUserNotInChannel},
		{"no channel", "CNOPE", human.ID, service.Content{Text: "x"}, service.ErrChannelNotFound},
		{"no content", ch, human.ID, service.Content{}, service.ErrNoText},
	}
	var want []service.Ephemeral
	for _, tt := range tests {
		ts, err := svc.PostEphemeral(t.Context(), bot, service.EphemeralOpts{Channel: tt.channel, User: tt.user, Content: tt.content})
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
			continue
		}
		if tt.wantErr == nil {
			want = append(want, service.Ephemeral{Channel: ch, User: human.ID, Sender: bot.ID, TS: ts, Content: tt.content})
		}
	}

	if diff := cmp.Diff(want, svc.Ephemerals()); diff != "" {
		t.Errorf("ring (-want +got):\n%s", diff)
	}
	after, err := svc.History(t.Context(), human, ch, service.PageOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if diff := cmp.Diff(before.Items, after.Items); diff != "" {
		t.Errorf("history changed by ephemeral (-before +after):\n%s", diff)
	}
	if got := c.got(); len(got) != 0 {
		t.Errorf("events = %v, want none", got)
	}
}

func TestEphemeralRingIsBounded(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	ch := generalID(t, svc, human)

	var last string
	for range service.EphemeralRingSize + 5 {
		ts, err := svc.PostEphemeral(t.Context(), bot, service.EphemeralOpts{Channel: ch, User: human.ID, Content: service.Content{Text: "x"}})
		if err != nil {
			t.Fatalf("PostEphemeral: %v", err)
		}
		last = ts
	}

	got := svc.Ephemerals()
	if len(got) != service.EphemeralRingSize || got[len(got)-1].TS != last {
		t.Errorf("ring len %d last %q, want %d ending in %q", len(got), got[len(got)-1].TS, service.EphemeralRingSize, last)
	}
	if got[0].TS >= got[1].TS {
		t.Errorf("ring not oldest-first: %q then %q", got[0].TS, got[1].TS)
	}
}
