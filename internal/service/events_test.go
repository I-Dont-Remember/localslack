package service_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

// collector is a subscriber that records events for assertions.
type collector struct {
	mu     sync.Mutex
	events []service.Event
}

func (c *collector) handle(e service.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) got() []service.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}

func TestPostEmitsMessagePostedOnce(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	ch := generalID(t, svc, human)
	var c collector
	svc.Subscribe(c.handle)

	msg, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: "hi"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	want := []service.Event{service.MessagePosted{
		Msg: msg,
		Scope: service.Scope{
			Channel: service.ChannelInfo{ID: ch, Name: "general", Kind: service.KindChannel},
			Targets: []string{bot.ID},
		},
	}}
	if diff := cmp.Diff(want, c.got()); diff != "" {
		t.Errorf("events (-want +got):\n%s", diff)
	}
}

func TestNoEventOnFailedPostOrRead(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, _ := usersOf(t, svc)
	ch := generalID(t, svc, human)
	var c collector
	svc.Subscribe(c.handle)

	tests := []struct {
		name string
		opts service.PostOpts
	}{
		{"no text", service.PostOpts{Channel: ch}},
		{"unknown channel", service.PostOpts{Channel: "CNOPE", Content: service.Content{Text: "x"}}},
		{"unknown thread", service.PostOpts{Channel: ch, ThreadTS: "1.000001", Content: service.Content{Text: "x"}}},
	}
	for _, tt := range tests {
		if _, err := svc.PostMessage(t.Context(), human, tt.opts); err == nil {
			t.Errorf("%s: PostMessage succeeded, want error", tt.name)
		}
	}
	if _, err := svc.History(t.Context(), human, ch, service.PageOpts{}); err != nil {
		t.Fatalf("History: %v", err)
	}
	if got := c.got(); len(got) != 0 {
		t.Errorf("events after failures and reads = %v, want none", got)
	}
}

func TestEmittedThreadReplyCarriesThread(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, _ := usersOf(t, svc)
	ch := generalID(t, svc, human)
	root, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: "root"}})
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	var c collector
	svc.Subscribe(c.handle)
	if _, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, ThreadTS: root.TS, Content: service.Content{Text: "r"}}); err != nil {
		t.Fatalf("reply: %v", err)
	}
	evs := c.got()
	if len(evs) != 1 {
		t.Fatalf("events = %v, want 1", evs)
	}
	if got := evs[0].(service.MessagePosted).Msg.ThreadTS; got != root.TS {
		t.Errorf("event ThreadTS = %q, want %q", got, root.TS)
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, _ := usersOf(t, svc)
	ch := generalID(t, svc, human)
	var kept, dropped collector
	svc.Subscribe(kept.handle)
	unsub := svc.Subscribe(dropped.handle)
	post := func() {
		t.Helper()
		if _, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: "x"}}); err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
	}

	post()
	unsub()
	unsub() // idempotent
	post()

	if k, d := len(kept.got()), len(dropped.got()); k != 2 || d != 1 {
		t.Errorf("events kept=%d dropped=%d, want 2 and 1", k, d)
	}
}

func TestConcurrentPostsObservedInTSOrder(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, _ := usersOf(t, svc)
	ch := generalID(t, svc, human)
	var c collector
	svc.Subscribe(c.handle)

	const n = 40
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: "x"}}); err != nil {
				t.Errorf("PostMessage: %v", err)
			}
		}()
	}
	wg.Wait()

	var got []string
	for _, e := range c.got() {
		got = append(got, e.(service.MessagePosted).Msg.TS)
	}
	if len(got) != n {
		t.Fatalf("events = %d, want %d", len(got), n)
	}
	if !slices.IsSorted(got) {
		t.Errorf("events not in ts order: %v", got)
	}
}
