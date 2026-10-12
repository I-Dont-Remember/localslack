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
		want    service.Principal
		wantErr error
	}{
		{"missing", "", service.Principal{}, service.ErrNotAuthed},
		{"bot token", "xoxb-anything", service.Principal{User: bot, TokenKind: service.TokenBot}, nil},
		{"user token", "xoxp-" + human.ID, service.Principal{User: human, TokenKind: service.TokenUser}, nil},
		{"unknown user", "xoxp-UNOPE", service.Principal{}, service.ErrInvalidAuth},
		{"unregistered app token", "xapp-1-x", service.Principal{}, service.ErrInvalidAuth},
		{"garbage", "hunter2", service.Principal{}, service.ErrInvalidAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.Authenticate(t.Context(), tt.token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("principal mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIdentify(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	tests := []struct {
		name  string
		token string
		want  service.Principal
	}{
		{"missing", "", service.Principal{}},
		{"bot", "xoxb-x", service.Principal{User: bot, TokenKind: service.TokenBot}},
		{"user", "xoxp-" + human.ID, service.Principal{User: human, TokenKind: service.TokenUser}},
		{"unknown user keeps kind", "xoxp-UNOPE", service.Principal{TokenKind: service.TokenUser}},
		{"app token", "xapp-1-x", service.Principal{TokenKind: service.TokenApp}},
		{"garbage", "hunter2", service.Principal{TokenKind: service.TokenInvalid}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, svc.Identify(t.Context(), tt.token)); diff != "" {
				t.Errorf("principal mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRegisteredApps(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	// Apps point at existing users; per-app bot users arrive with the registry.
	app := service.App{ID: "A1", Name: "one", BotToken: "xoxb-app1", AppToken: "xapp-app1", BotUserID: human.ID, Scopes: []string{"chat:write"}}
	svc.SetApps(app)
	ctx := t.Context()

	got, err := svc.Authenticate(ctx, "xoxb-app1")
	want := service.Principal{User: human, App: &app, TokenKind: service.TokenBot, Scopes: []string{"chat:write"}}
	if err != nil || cmp.Diff(want, got) != "" {
		t.Errorf("registered bot = %+v, %v; want %+v", got, err, want)
	}
	got, err = svc.Authenticate(ctx, "xapp-app1")
	want = service.Principal{App: &app, TokenKind: service.TokenApp, Scopes: []string{"chat:write"}}
	if err != nil || cmp.Diff(want, got) != "" {
		t.Errorf("app token = %+v, %v; want %+v", got, err, want)
	}
	got, err = svc.Authenticate(ctx, "xoxb-other")
	if err != nil || got.App != nil || got.User != bot {
		t.Errorf("unregistered bot = %+v, %v; want default bot, no app", got, err)
	}

	svc.InvalidateApps()
	got, err = svc.Authenticate(ctx, "xoxb-app1")
	if err != nil || got.App != nil || got.User != bot {
		t.Errorf("after invalidate = %+v, %v; want default bot, no app", got, err)
	}
	if _, err := svc.Authenticate(ctx, "xapp-app1"); !errors.Is(err, service.ErrInvalidAuth) {
		t.Errorf("app token after invalidate err = %v, want ErrInvalidAuth", err)
	}
}

func TestDefaultBot(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	_, bot := usersOf(t, svc)
	got, err := svc.DefaultBot(t.Context())
	if want := (service.Principal{User: bot, TokenKind: service.TokenBot}); err != nil || cmp.Diff(want, got) != "" {
		t.Errorf("DefaultBot = %+v, %v; want %+v", got, err, want)
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

	page, err := svc.History(t.Context(), caller, ch, service.PageOpts{Limit: 2})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if got := []string{page.Items[0].TS, page.Items[1].TS}; !slices.Equal(got, []string{posted[2], posted[1]}) {
		t.Errorf("first page = %v, want newest first %v", got, []string{posted[2], posted[1]})
	}
	if page.NextCursor == "" {
		t.Fatal("expected a next cursor")
	}
	next, err := svc.History(t.Context(), caller, ch, service.PageOpts{Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("History next: %v", err)
	}
	if len(next.Items) == 0 || next.Items[0].TS >= posted[1] {
		t.Errorf("second page starts at %v, want older than %s", next.Items, posted[1])
	}
	if _, err := svc.History(t.Context(), caller, "CNOPE", service.PageOpts{}); !errors.Is(err, service.ErrChannelNotFound) {
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

	replies, err := svc.Replies(t.Context(), caller, ch, parent.TS, service.PageOpts{})
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

	hist, err := svc.History(t.Context(), caller, ch, service.PageOpts{Limit: 1})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist.Items) != 1 || hist.Items[0].TS != parent.TS {
		t.Errorf("history top = %+v, want only the parent (replies excluded)", hist.Items)
	}
	if _, err := svc.Replies(t.Context(), caller, ch, "1.000001", service.PageOpts{}); !errors.Is(err, service.ErrThreadNotFound) {
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
	h, err := svc.History(t.Context(), caller, im.ID, service.PageOpts{})
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
		t.Fatalf("pinned seed has %d users (err %v); the mpim test needs 3, pick another PinnedSeed", len(us), err)
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

func TestRuntimeIDsDoNotCollideAcrossRestarts(t *testing.T) {
	t.Parallel()

	st := storetest.NewStore(t)
	for i := range 3 { // each "process start" must mint fresh ids
		svc, err := service.New(t.Context(), st, fixedClock(), "")
		if err != nil {
			t.Fatalf("New #%d: %v", i, err)
		}
		caller, _ := usersOf(t, svc)
		if _, err := svc.CreateChannel(t.Context(), caller, "restart-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("CreateChannel after restart #%d: %v", i, err)
		}
	}
}

func TestCursorsAreBoundToTheirEndpoint(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	ch := generalID(t, svc, caller)
	for range 3 {
		if _, err := svc.PostMessage(t.Context(), caller, ch, "x", ""); err != nil {
			t.Fatalf("post: %v", err)
		}
	}
	hist, err := svc.History(t.Context(), caller, ch, service.PageOpts{Limit: 1})
	if err != nil || hist.NextCursor == "" {
		t.Fatalf("history page: %+v, %v", hist, err)
	}
	if _, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Cursor: hist.NextCursor}); !errors.Is(err, service.ErrInvalidCursor) {
		t.Errorf("history cursor on conversations.list err = %v, want ErrInvalidCursor", err)
	}
}

func TestAuthenticateEdges(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	for _, tok := range []string{"xoxp-", "xoxp"} {
		if _, err := svc.Authenticate(t.Context(), tok); !errors.Is(err, service.ErrInvalidAuth) {
			t.Errorf("Authenticate(%q) err = %v, want ErrInvalidAuth", tok, err)
		}
	}
	if _, err := svc.Authenticate(t.Context(), "xoxb-"); err != nil {
		t.Errorf("bare xoxb- err = %v, want nil (any bot token)", err)
	}
}

func TestConversationShape(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, other := usersOf(t, svc)
	all, err := svc.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}

	ch, err := svc.CreateChannel(t.Context(), caller, "shape")
	if err != nil || !ch.IsMember || ch.NumMembers != 1 {
		t.Errorf("created channel = %+v, %v; want IsMember and 1 member", ch, err)
	}
	im, err := svc.OpenDM(t.Context(), caller, []string{other.ID})
	if err != nil || im.Peer != other.ID || im.NumMembers != 2 {
		t.Errorf("im = %+v, %v; want Peer %s, 2 members", im, err, other.ID)
	}
	self, err := svc.OpenDM(t.Context(), caller, []string{caller.ID})
	if err != nil || self.Peer != caller.ID {
		t.Errorf("self im = %+v, %v; want Peer %s", self, err, caller.ID)
	}

	general := generalID(t, svc, caller)
	got, err := svc.Conversation(t.Context(), caller, general)
	if err != nil || !got.IsMember || got.NumMembers != len(all) {
		t.Errorf("#general = %+v, %v; want member of %d", got, err, len(all))
	}
	if _, err := svc.Conversation(t.Context(), caller, "CNOPE"); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("missing conversation err = %v", err)
	}
}

func TestMessageThreadFields(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, other := usersOf(t, svc)
	ch := generalID(t, svc, caller)
	plain, _ := svc.PostMessage(t.Context(), caller, ch, "plain", "")
	root, _ := svc.PostMessage(t.Context(), caller, ch, "root", "")
	if _, err := svc.PostMessage(t.Context(), other, ch, "reply", root.TS); err != nil {
		t.Fatalf("reply: %v", err)
	}

	hist, err := svc.History(t.Context(), caller, ch, service.PageOpts{Limit: 2})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	byTS := map[string]service.Message{}
	for _, m := range hist.Items {
		byTS[m.TS] = m
	}
	if got := byTS[plain.TS].ThreadTS; got != "" {
		t.Errorf("plain.ThreadTS = %q, want empty", got)
	}
	if got := byTS[root.TS]; got.ThreadTS != root.TS || got.ReplyCount != 1 {
		t.Errorf("root = %+v, want ThreadTS == own ts and 1 reply", got)
	}

	th, err := svc.Replies(t.Context(), caller, ch, root.TS, service.PageOpts{})
	if err != nil || len(th.Items) != 2 {
		t.Fatalf("Replies = %+v, %v", th.Items, err)
	}
	if r := th.Items[1]; r.ParentUserID != caller.ID || r.ThreadTS != root.TS {
		t.Errorf("reply = %+v, want ParentUserID %s ThreadTS %s", r, caller.ID, root.TS)
	}
	if th.Items[0].ParentUserID != "" {
		t.Errorf("root.ParentUserID = %q, want empty", th.Items[0].ParentUserID)
	}
}

func TestHistoryRange(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	ch := generalID(t, svc, caller)
	var ts []string
	for _, text := range []string{"a", "b", "c"} {
		m, err := svc.PostMessage(t.Context(), caller, ch, text, "")
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		ts = append(ts, m.TS)
	}
	tests := []struct {
		name string
		opts service.PageOpts
		want []string
	}{
		{"exclusive bounds", service.PageOpts{Oldest: ts[0], Latest: ts[2]}, []string{ts[1]}},
		{"inclusive bounds", service.PageOpts{Oldest: ts[0], Latest: ts[2], Inclusive: true}, []string{ts[2], ts[1], ts[0]}},
		{"oldest only", service.PageOpts{Oldest: ts[1]}, []string{ts[2]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			page, err := svc.History(t.Context(), caller, ch, tt.opts)
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			var got []string
			for _, m := range page.Items {
				got = append(got, m.TS)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestArchive(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, _ := usersOf(t, svc)
	general := generalID(t, svc, caller)
	ch, err := svc.CreateChannel(t.Context(), caller, "doomed")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.ArchiveChannel(t.Context(), caller, general); !errors.Is(err, service.ErrCantArchiveGeneral) {
		t.Errorf("archive general err = %v, want ErrCantArchiveGeneral", err)
	}
	if err := svc.ArchiveChannel(t.Context(), caller, "CNOPE"); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("archive missing err = %v, want ErrChannelNotFound", err)
	}
	if err := svc.ArchiveChannel(t.Context(), caller, ch.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := svc.ArchiveChannel(t.Context(), caller, ch.ID); !errors.Is(err, service.ErrAlreadyArchived) {
		t.Errorf("second archive err = %v, want ErrAlreadyArchived", err)
	}
	if _, err := svc.PostMessage(t.Context(), caller, ch.ID, "hi", ""); !errors.Is(err, service.ErrIsArchived) {
		t.Errorf("post to archived err = %v, want ErrIsArchived", err)
	}

	for _, tc := range []struct {
		exclude bool
		present bool
	}{{false, true}, {true, false}} {
		page, err := svc.ListConversations(t.Context(), caller, service.ListOpts{ExcludeArchived: tc.exclude, Limit: 1000})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		found := false
		for _, c := range page.Items {
			found = found || c.ID == ch.ID
		}
		if found != tc.present {
			t.Errorf("exclude_archived=%v: archived channel listed=%v, want %v", tc.exclude, found, tc.present)
		}
	}
}

func TestConcurrentCreateAndPostAndRead(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, other := usersOf(t, svc)
	ch := generalID(t, svc, caller)

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := svc.CreateChannel(t.Context(), caller, "race-"+strconv.Itoa(i)); err != nil {
				t.Errorf("create: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := svc.OpenDM(t.Context(), caller, []string{other.ID}); err != nil {
				t.Errorf("open dm: %v", err)
			}
			if _, err := svc.PostMessage(t.Context(), caller, ch, "r", ""); err != nil {
				t.Errorf("post: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := svc.History(t.Context(), caller, ch, service.PageOpts{}); err != nil {
				t.Errorf("history: %v", err)
			}
		}()
	}
	wg.Wait()

	ims, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Types: []string{"im"}})
	if err != nil || len(ims.Items) != 1 {
		t.Errorf("concurrent OpenDM made %d ims, %v; want exactly 1", len(ims.Items), err)
	}
}

func TestListAll(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	caller, other := usersOf(t, svc)
	for i := range 3 {
		if _, err := svc.CreateChannel(t.Context(), caller, "all-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if _, err := svc.OpenDM(t.Context(), caller, []string{other.ID}); err != nil {
		t.Fatalf("open dm: %v", err)
	}

	big, err := svc.ListConversations(t.Context(), caller, service.ListOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got, err := svc.ListAll(t.Context(), caller, []string{"public_channel"}, false)
	if err != nil || len(got) != len(big.Items) {
		t.Errorf("ListAll channels = %d, %v; want %d", len(got), err, len(big.Items))
	}
	dms, err := svc.ListAll(t.Context(), caller, []string{"im", "mpim"}, false)
	if err != nil || len(dms) != 1 || dms[0].Peer != other.ID {
		t.Errorf("ListAll dms = %+v, %v; want one im with peer %s", dms, err, other.ID)
	}
}
