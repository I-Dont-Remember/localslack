package service_test

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
	"localslack/internal/store"
	"localslack/internal/storetest"
)

func fixedClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
}

// newSvc returns a service over a seeded store plus a human caller, another human-or-bot, and #general.
func newSvc(t *testing.T) (*service.Service, *store.Store) {
	t.Helper()
	st := storetest.NewStore(t)
	svc, err := service.New(t.Context(), st, fixedClock(), "svc-ids")
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc, st
}

func usersOf(t *testing.T, svc *service.Service) (caller, other service.User) {
	t.Helper()
	us, err := svc.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	for _, u := range us {
		switch {
		case caller.ID == "" && !u.IsBot:
			caller = u
		case other.ID == "" && u.IsBot:
			other = u
		}
	}
	if caller.ID == "" || other.ID == "" {
		t.Fatalf("seed lacks a human and a bot: %+v", us)
	}
	return caller, other
}

func generalID(t *testing.T, svc *service.Service, caller service.User) string {
	t.Helper()
	page, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	for _, c := range page.Items {
		if c.Name == "general" {
			return c.ID
		}
	}
	t.Fatal("no #general")
	return ""
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	tests := []struct {
		name    string
		token   string
		wantID  string
		wantErr error
	}{
		{"missing", "", "", service.ErrNotAuthed},
		{"bot token", "xoxb-anything", bot.ID, nil},
		{"user token", "xoxp-" + human.ID, human.ID, nil},
		{"unknown user", "xoxp-UNOPE", "", service.ErrInvalidAuth},
		{"garbage", "hunter2", "", service.ErrInvalidAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.Authenticate(t.Context(), tt.token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.ID != tt.wantID {
				t.Errorf("user = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestCreateChannel(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"ok", "project-x", nil},
		{"duplicate", "project-x", service.ErrNameTaken},
		{"duplicate ignores case in lookup", "project-x", service.ErrNameTaken},
		{"general exists from seed", "general", service.ErrNameTaken},
		{"uppercase", "Project", service.ErrInvalidName},
		{"space", "my channel", service.ErrInvalidName},
		{"empty", "", service.ErrInvalidName},
		{"too long", string(make([]byte, 81)), service.ErrInvalidName},
		{"leading hyphen", "-x", service.ErrInvalidName},
	}
	// Sequential: later cases depend on earlier ones.
	for _, tt := range tests {
		got, err := svc.CreateChannel(t.Context(), caller, tt.in)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
			continue
		}
		if tt.wantErr == nil && (got.Name != tt.in || got.Kind != service.KindChannel || got.Creator != caller.ID) {
			t.Errorf("%s: got %+v", tt.name, got)
		}
	}
}

func TestListConversationsPagination(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	for i := range 5 {
		if _, err := svc.CreateChannel(t.Context(), caller, "extra-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("CreateChannel: %v", err)
		}
	}
	all, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Limit: 1000})
	if err != nil || all.NextCursor != "" {
		t.Fatalf("unpaged = %d items, cursor %q, err %v", len(all.Items), all.NextCursor, err)
	}

	var walked []string
	cursor := ""
	for range 100 {
		page, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(page.Items) > 2 {
			t.Fatalf("page has %d items, limit 2", len(page.Items))
		}
		for _, c := range page.Items {
			walked = append(walked, c.ID)
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	var want []string
	for _, c := range all.Items {
		want = append(want, c.ID)
	}
	if diff := cmp.Diff(want, walked); diff != "" {
		t.Errorf("paged walk != unpaged (-want +got):\n%s", diff)
	}

	if _, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Cursor: "!!not-a-cursor"}); !errors.Is(err, service.ErrInvalidCursor) {
		t.Errorf("bad cursor err = %v, want ErrInvalidCursor", err)
	}
	if _, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Types: []string{"bogus"}}); !errors.Is(err, service.ErrInvalidTypes) {
		t.Errorf("bad types err = %v, want ErrInvalidTypes", err)
	}
}

func TestPostMessageAndHistory(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	ch := generalID(t, svc, caller)

	if _, err := svc.PostMessage(t.Context(), caller, "CNOPE", "x", ""); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("missing channel err = %v", err)
	}
	if _, err := svc.PostMessage(t.Context(), caller, ch, "", ""); !errors.Is(err, service.ErrNoText) {
		t.Errorf("empty text err = %v", err)
	}

	var posted []string
	for _, text := range []string{"one", "two", "three"} {
		m, err := svc.PostMessage(t.Context(), caller, ch, text, "")
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		posted = append(posted, m.TS)
	}
	if !slices.IsSorted(posted) || len(slices.Compact(slices.Clone(posted))) != 3 {
		t.Errorf("ts not strictly increasing: %v", posted)
	}

	page, err := svc.History(t.Context(), ch, service.PageOpts{Limit: 2})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if got := []string{page.Items[0].TS, page.Items[1].TS}; !slices.Equal(got, []string{posted[2], posted[1]}) {
		t.Errorf("first page = %v, want newest first %v", got, []string{posted[2], posted[1]})
	}
	if page.NextCursor == "" {
		t.Fatal("expected a next cursor")
	}
	next, err := svc.History(t.Context(), ch, service.PageOpts{Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("History next: %v", err)
	}
	if len(next.Items) == 0 || next.Items[0].TS >= posted[1] {
		t.Errorf("second page starts at %v, want older than %s", next.Items, posted[1])
	}
	if _, err := svc.History(t.Context(), "CNOPE", service.PageOpts{}); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("history of missing channel err = %v", err)
	}
}

func TestThreads(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	ch := generalID(t, svc, caller)

	parent, err := svc.PostMessage(t.Context(), caller, ch, "parent", "")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	reply, err := svc.PostMessage(t.Context(), caller, ch, "reply", parent.TS)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	// Replying to a reply attaches to the thread root.
	reply2, err := svc.PostMessage(t.Context(), caller, ch, "reply to reply", reply.TS)
	if err != nil {
		t.Fatalf("reply2: %v", err)
	}
	if reply2.ThreadTS != parent.TS {
		t.Errorf("reply2.ThreadTS = %q, want root %q", reply2.ThreadTS, parent.TS)
	}
	if _, err := svc.PostMessage(t.Context(), caller, ch, "orphan", "1.000001"); !errors.Is(err, service.ErrThreadNotFound) {
		t.Errorf("orphan err = %v, want ErrThreadNotFound", err)
	}

	replies, err := svc.Replies(t.Context(), ch, parent.TS, service.PageOpts{})
	if err != nil {
		t.Fatalf("Replies: %v", err)
	}
	var got []string
	for _, m := range replies.Items {
		got = append(got, m.TS)
	}
	if want := []string{parent.TS, reply.TS, reply2.TS}; !slices.Equal(got, want) {
		t.Errorf("replies = %v, want %v", got, want)
	}
	if p := replies.Items[0]; p.ReplyCount != 2 || p.LatestReply != reply2.TS {
		t.Errorf("parent = %+v, want reply_count 2 latest %s", p, reply2.TS)
	}

	hist, err := svc.History(t.Context(), ch, service.PageOpts{Limit: 1})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist.Items) != 1 || hist.Items[0].TS != parent.TS {
		t.Errorf("history top = %+v, want only the parent (replies excluded)", hist.Items)
	}
	if _, err := svc.Replies(t.Context(), ch, "1.000001", service.PageOpts{}); !errors.Is(err, service.ErrThreadNotFound) {
		t.Errorf("replies of missing thread err = %v", err)
	}
}

func TestOpenDM(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, other := usersOf(t, svc)

	im, err := svc.OpenDM(t.Context(), caller, []string{other.ID})
	if err != nil || im.Kind != service.KindIM || im.ID[0] != 'D' {
		t.Fatalf("OpenDM = %+v, %v; want a D... im", im, err)
	}
	again, err := svc.OpenDM(t.Context(), caller, []string{other.ID})
	if err != nil || again.ID != im.ID {
		t.Errorf("reopen = %q, %v; want same %q", again.ID, err, im.ID)
	}
	reverse, err := svc.OpenDM(t.Context(), other, []string{caller.ID})
	if err != nil || reverse.ID != im.ID {
		t.Errorf("reverse open = %q, %v; want same %q", reverse.ID, err, im.ID)
	}
	self, err := svc.OpenDM(t.Context(), caller, []string{caller.ID})
	if err != nil || self.Kind != service.KindIM || self.ID == im.ID {
		t.Errorf("self dm = %+v, %v; want distinct im", self, err)
	}
	if _, err := svc.OpenDM(t.Context(), caller, []string{"UNOPE"}); !errors.Is(err, service.ErrUserNotFound) {
		t.Errorf("unknown user err = %v", err)
	}
	if _, err := svc.OpenDM(t.Context(), caller, nil); !errors.Is(err, service.ErrNoUsers) {
		t.Errorf("no users err = %v", err)
	}

	if _, err := svc.PostMessage(t.Context(), caller, im.ID, "psst", ""); err != nil {
		t.Fatalf("post to dm: %v", err)
	}
	h, err := svc.History(t.Context(), im.ID, service.PageOpts{})
	if err != nil || len(h.Items) != 1 || h.Items[0].Text != "psst" {
		t.Errorf("dm history = %+v, %v", h.Items, err)
	}

	listed, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Types: []string{"im"}})
	if err != nil || len(listed.Items) != 2 {
		t.Errorf("caller im list = %d, %v; want 2 (dm + self)", len(listed.Items), err)
	}
}

func TestOpenGroupDM(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	us, err := svc.Users(t.Context())
	if err != nil || len(us) < 3 {
		t.Skipf("pinned seed has %d users; need 3 for an mpim (err %v)", len(us), err)
	}
	mp, err := svc.OpenDM(t.Context(), us[0], []string{us[1].ID, us[2].ID})
	if err != nil || mp.Kind != service.KindMPIM || mp.ID[0] != 'G' {
		t.Errorf("OpenDM = %+v, %v; want a G... mpim", mp, err)
	}
}

func TestTSUniqueMonotonicUnderConcurrency(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	caller, _ := usersOf(t, svc)
	ch := generalID(t, svc, caller)

	const workers, perWorker = 8, 15
	var (
		mu  sync.Mutex
		all []string
		wg  sync.WaitGroup
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mine []string
			for range perWorker {
				m, err := svc.PostMessage(t.Context(), caller, ch, "x", "")
				if err != nil {
					t.Errorf("PostMessage: %v", err)
					return
				}
				mine = append(mine, m.TS)
			}
			if !slices.IsSorted(mine) {
				t.Errorf("one writer saw non-increasing ts: %v", mine)
			}
			mu.Lock()
			all = append(all, mine...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if got := len(slices.Compact(slices.Sorted(slices.Values(all)))); got != workers*perWorker {
		t.Errorf("unique ts = %d, want %d", got, workers*perWorker)
	}
	gotMax, err := st.MaxTS(t.Context())
	if err != nil || gotMax != slices.Max(all) {
		t.Errorf("MaxTS = %q, %v; want %q", gotMax, err, slices.Max(all))
	}
}

func TestTSSurvivesRestartWithEarlierClock(t *testing.T) {
	t.Parallel()

	st := storetest.NewStore(t)
	late := func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	first, err := service.New(t.Context(), st, late, "a")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	caller, _ := usersOf(t, first)
	ch := generalID(t, first, caller)
	m1, err := first.PostMessage(t.Context(), caller, ch, "before restart", "")
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	early := func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	second, err := service.New(t.Context(), st, early, "b")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m2, err := second.PostMessage(t.Context(), caller, ch, "after restart", "")
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if m2.TS <= m1.TS {
		t.Errorf("ts after restart %s <= before %s", m2.TS, m1.TS)
	}
}
