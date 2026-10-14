package service_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

// threadFixture posts a root with two replies by human in #general.
type threadFixture struct {
	svc              *service.Service
	human, bot       service.User
	ch               string
	root, rep1, rep2 service.Message
	events           *collector
	history, replies func() []service.Message
	deleteAs         func(service.User, string) error
}

func newThread(t *testing.T) *threadFixture {
	t.Helper()
	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	f := &threadFixture{svc: svc, human: human, bot: bot, ch: generalID(t, svc, human), events: &collector{}}
	svc.Subscribe(f.events.handle)
	post := func(text, thread string) service.Message {
		m, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: f.ch, ThreadTS: thread, Content: service.Content{Text: text}})
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		return m
	}
	f.root = post("root", "")
	f.rep1 = post("one", f.root.TS)
	f.rep2 = post("two", f.root.TS)
	f.history = func() []service.Message {
		p, err := svc.History(t.Context(), human, f.ch, service.PageOpts{Limit: 1000})
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		return p.Items
	}
	f.replies = func() []service.Message {
		p, err := svc.Replies(t.Context(), human, f.ch, f.root.TS, service.PageOpts{Limit: 1000})
		if err != nil {
			t.Fatalf("Replies: %v", err)
		}
		return p.Items
	}
	f.deleteAs = func(u service.User, ts string) error { return svc.DeleteMessage(t.Context(), u, f.ch, ts) }
	return f
}

func tsList(ms []service.Message) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.TS)
	}
	return out
}

func TestDeleteReplyUpdatesRoot(t *testing.T) {
	t.Parallel()

	f := newThread(t)
	if err := f.deleteAs(f.human, f.rep2.TS); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	got := f.history()[0]
	if got.ReplyCount != 1 || got.LatestReply != f.rep1.TS {
		t.Errorf("root after deleting latest reply: count %d latest %q, want 1 and %q", got.ReplyCount, got.LatestReply, f.rep1.TS)
	}
	if diff := cmp.Diff([]string{f.root.TS, f.rep1.TS}, tsList(f.replies())); diff != "" {
		t.Errorf("thread (-want +got):\n%s", diff)
	}

	if err := f.deleteAs(f.human, f.rep1.TS); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	got = f.history()[0]
	if got.ReplyCount != 0 || got.LatestReply != "" || got.ThreadTS != "" {
		t.Errorf("root with no replies left = %+v, want plain message", got)
	}
}

func TestDeleteRootWithRepliesLeavesTombstone(t *testing.T) {
	t.Parallel()

	f := newThread(t)
	if err := f.deleteAs(f.human, f.root.TS); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	got := f.history()[0]
	want := service.Message{
		ChannelID: f.ch, TS: f.root.TS, UserID: f.human.ID, Text: "This message was deleted.", Subtype: "tombstone",
		ThreadTS: f.root.TS, ReplyCount: 2, LatestReply: f.rep2.TS,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("tombstone (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{f.root.TS, f.rep1.TS, f.rep2.TS}, tsList(f.replies())); diff != "" {
		t.Errorf("replies stay readable (-want +got):\n%s", diff)
	}
	if err := f.deleteAs(f.human, f.root.TS); !errors.Is(err, service.ErrMessageNotFound) {
		t.Errorf("deleting a tombstone = %v, want ErrMessageNotFound", err)
	}

	for _, ts := range []string{f.rep1.TS, f.rep2.TS} {
		if err := f.deleteAs(f.human, ts); err != nil {
			t.Fatalf("DeleteMessage reply: %v", err)
		}
	}
	if left := f.history(); slices.Contains(tsList(left), f.root.TS) {
		t.Errorf("tombstone still in history after its last reply went: %v", tsList(left))
	}
}

func TestDeleteMessageErrorsAndEvents(t *testing.T) {
	t.Parallel()

	f := newThread(t)
	solo, err := f.svc.PostMessage(t.Context(), f.human, service.PostOpts{Channel: f.ch, Content: service.Content{Text: "solo"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	tests := []struct {
		name    string
		actor   service.User
		ts      string
		wantErr error
	}{
		{"not author", f.bot, solo.TS, service.ErrCantDeleteMessage},
		{"no message", f.human, "1.000001", service.ErrMessageNotFound},
		{"ok", f.human, solo.TS, nil},
		{"already gone", f.human, solo.TS, service.ErrMessageNotFound},
	}
	for _, tt := range tests {
		if err := f.deleteAs(tt.actor, tt.ts); !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
		}
	}
	if err := f.svc.DeleteMessage(t.Context(), f.human, "CNOPE", solo.TS); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("unknown channel: err = %v, want ErrChannelNotFound", err)
	}

	var deleted []string
	for _, e := range f.events.got() {
		if ev, ok := e.(service.MessageDeleted); ok {
			deleted = append(deleted, ev.Msg.TS)
		}
	}
	if diff := cmp.Diff([]string{solo.TS}, deleted); diff != "" {
		t.Errorf("MessageDeleted events (-want +got):\n%s", diff)
	}
}
