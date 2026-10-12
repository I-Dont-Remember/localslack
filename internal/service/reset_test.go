package service_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
	"localslack/internal/store"
)

// addApp stores an app and its bot user, as the app registry will.
func addApp(t *testing.T, svc *service.Service, st *store.Store, id string) service.App {
	t.Helper()
	team, err := svc.Team(t.Context())
	if err != nil {
		t.Fatalf("Team: %v", err)
	}
	botUser := id + "BOTUSER"
	err = st.Write(t.Context(), func(tx *store.Tx) error {
		if err := tx.InsertUser(t.Context(), store.User{ID: botUser, TeamID: team.ID, Name: "appbot-" + id, IsBot: true, BotID: "B" + id}); err != nil {
			return err
		}
		return tx.InsertApp(t.Context(), store.App{ID: id, Name: id, BotToken: "xoxb-" + id, AppToken: "xapp-" + id, BotUserID: botUser, BotID: "B" + id})
	})
	if err != nil {
		t.Fatalf("add app: %v", err)
	}
	if err := svc.RefreshApps(t.Context()); err != nil {
		t.Fatalf("RefreshApps: %v", err)
	}
	return service.App{ID: id, BotToken: "xoxb-" + id, BotUserID: botUser}
}

func userIDs(t *testing.T, svc *service.Service) []string {
	t.Helper()
	us, err := svc.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	out := []string{}
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func TestResetSeedIsReproducible(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()

	first, err := svc.Reset(ctx, service.ResetOpts{Seed: "repro"})
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	idsFirst := userIDs(t, svc)
	if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "other"}); err != nil {
		t.Fatalf("Reset other: %v", err)
	}
	second, err := svc.Reset(ctx, service.ResetOpts{Seed: "repro"})
	if err != nil {
		t.Fatalf("Reset again: %v", err)
	}

	if diff := cmp.Diff(idsFirst, userIDs(t, svc)); diff != "" {
		t.Errorf("same seed, different user ids (-first +second):\n%s", diff)
	}
	if first != second || first.Seed != "repro" || first.Counts.Users == 0 || first.Counts.Channels == 0 || first.Counts.Messages == 0 {
		t.Errorf("results %+v / %+v, want equal, seed echoed, non-empty counts", first, second)
	}
}

func TestResetKeepsTeamAndReseedsRuntimeIDs(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	teamBefore, err := svc.Team(ctx)
	if err != nil {
		t.Fatalf("Team: %v", err)
	}
	newChannel := func() string {
		t.Helper()
		if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "rt"}); err != nil {
			t.Fatalf("Reset: %v", err)
		}
		caller, _ := usersOf(t, svc)
		c, err := svc.CreateChannel(ctx, caller, "fresh-channel")
		if err != nil {
			t.Fatalf("CreateChannel: %v", err)
		}
		return c.ID
	}

	a, b := newChannel(), newChannel()

	if a != b {
		t.Errorf("channel id after identical resets: %q vs %q, want equal (runtime ids reseeded)", a, b)
	}
	if teamAfter, err := svc.Team(ctx); err != nil || teamAfter != teamBefore {
		t.Errorf("team after reset = %+v, %v; want unchanged %+v", teamAfter, err, teamBefore)
	}
}

func TestResetKeepsTsMonotonic(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	caller, _ := usersOf(t, svc)
	before, err := svc.PostMessage(ctx, caller, service.PostOpts{Channel: generalID(t, svc, caller), Content: service.Content{Text: "pre"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "after"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	caller, _ = usersOf(t, svc)
	after, err := svc.PostMessage(ctx, caller, service.PostOpts{Channel: generalID(t, svc, caller), Content: service.Content{Text: "post"}})
	if err != nil {
		t.Fatalf("PostMessage after reset: %v", err)
	}

	if after.TS <= before.TS {
		t.Errorf("ts after reset %q <= before %q, want strictly greater", after.TS, before.TS)
	}
}

func TestResetEmitsNoEvents(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	var n atomic.Int32
	svc.Subscribe(func(service.Event) { n.Add(1) })

	if _, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "quiet"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if got := n.Load(); got != 0 {
		t.Errorf("events during reset = %d, want 0", got)
	}
}

func TestResetAppsPartialVersusAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		all          bool
		wantApp      bool
		wantBotUser  bool
		wantTokenApp bool
	}{
		{"partial keeps apps and their bot users", false, true, true, true},
		{"all removes them", true, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, st := newSvc(t)
			ctx := t.Context()
			app := addApp(t, svc, st, "A1")

			if _, err := svc.Reset(ctx, service.ResetOpts{All: tt.all, Seed: "apps"}); err != nil {
				t.Fatalf("Reset: %v", err)
			}

			apps, err := st.Apps(ctx)
			if err != nil {
				t.Fatalf("Apps: %v", err)
			}
			if got := len(apps) == 1; got != tt.wantApp {
				t.Errorf("app kept = %v, want %v", got, tt.wantApp)
			}
			_, userErr := st.User(ctx, app.BotUserID)
			if got := userErr == nil; got != tt.wantBotUser {
				t.Errorf("bot user kept = %v (%v), want %v", got, userErr, tt.wantBotUser)
			}
			if got := svc.Identify(ctx, app.BotToken).App != nil; got != tt.wantTokenApp {
				t.Errorf("bot token still maps to app = %v, want %v (cache refreshed)", got, tt.wantTokenApp)
			}
		})
	}
}

func TestResetFailureLeavesStateUntouched(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	ctx := t.Context()
	if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "collide"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	// An app whose bot user id is a user the same seed generates again.
	users := userIDs(t, svc)
	err := st.Write(ctx, func(tx *store.Tx) error {
		return tx.InsertApp(ctx, store.App{ID: "A9", BotUserID: users[0], Name: "clash"})
	})
	if err != nil {
		t.Fatalf("insert app: %v", err)
	}

	_, err = svc.Reset(ctx, service.ResetOpts{Seed: "collide"})

	if !errors.Is(err, service.ErrResetIDCollision) {
		t.Fatalf("Reset err = %v, want ErrResetIDCollision", err)
	}
	if diff := cmp.Diff(users, userIDs(t, svc)); diff != "" {
		t.Errorf("users changed by failed reset (-before +after):\n%s", diff)
	}
}

func TestResetDuringConcurrentPosts(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				caller, _ := usersOf(t, svc)
				ch := generalID(t, svc, caller)
				_, err := svc.PostMessage(ctx, caller, service.PostOpts{Channel: ch, Content: service.Content{Text: "load"}})
				// The workspace may be wiped between lookup and post: only domain errors are fine.
				if err != nil && !errors.Is(err, service.ErrChannelNotFound) {
					t.Errorf("worker %d: PostMessage: %v", w, err)
					return
				}
			}
		}()
	}
	for i := range 5 {
		if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "storm"}); err != nil {
			t.Errorf("Reset %d: %v", i, err)
		}
	}
	wg.Wait()

	caller, _ := usersOf(t, svc)
	if _, err := svc.PostMessage(ctx, caller, service.PostOpts{Channel: generalID(t, svc, caller), Content: service.Content{Text: "alive"}}); err != nil {
		t.Errorf("post after storm: %v", err)
	}
}
