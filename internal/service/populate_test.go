package service_test

import (
	"encoding/json"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

// handFixture is a small hand-written workspace with fixed ids, in export order
// (users, channels, dms by id; messages by conversation then ts; members sorted).
func handFixture() scenario.Fixture {
	return scenario.Fixture{
		Version: 1,
		Team:    &scenario.Team{ID: "TACME", Name: "Acme", Domain: "acme"},
		Users: []scenario.User{
			{ID: "UANN", Name: "ann", RealName: "Ann Example"},
			{ID: "UBOB", Name: "bob", RealName: "Bob Example"},
			{ID: "UBOT", Name: "acmebot", RealName: "Acme Bot", IsBot: true, BotID: "BACME"},
		},
		Channels: []scenario.Channel{
			{ID: "CGEN", Name: "general", Creator: "UANN", Created: 1700000000, Members: []string{"UANN", "UBOB", "UBOT"}},
			{ID: "CRET", Name: "retired", Creator: "UBOB", Created: 1700000001, IsArchived: true, Members: []string{"UBOB"}},
			{ID: "GSEC", Name: "secret", Creator: "UANN", Created: 1700000002, IsPrivate: true, Members: []string{"UANN", "UBOB"}},
		},
		DMs: []scenario.DM{{ID: "DANNBOB", Creator: "UANN", Created: 1700000003, Members: []string{"UANN", "UBOB"}}},
		Messages: []scenario.Message{
			{Channel: "CGEN", TS: "1700000100.000001", User: "UANN", Text: "root"},
			{
				Channel: "CGEN", TS: "1700000200.000002", User: "UBOB", Text: "reply", ThreadTS: "1700000100.000001",
				Reactions: []scenario.Reaction{{Name: "eyes", Users: []string{"UANN", "UBOT"}}, {Name: "tada", Users: []string{"UBOB"}}},
			},
			{
				Channel: "CGEN", TS: "1700000300.000003", User: "UBOT", BotID: "BACME", Text: "rich", Subtype: "bot_message",
				Blocks:   json.RawMessage(`[{"type":"divider"}]`),
				Metadata: json.RawMessage(`{"event_payload":{"k":1},"event_type":"x"}`),
				Edited:   &scenario.Edit{User: "UBOT", TS: "1700000400.000004"},
			},
			{Channel: "DANNBOB", TS: "1700000500.000005", User: "UANN", Text: "psst"},
			{Channel: "GSEC", TS: "1700000600.000006", User: "UBOB", Text: "hush"},
		},
	}
}

func resetTo(t *testing.T, svc *service.Service, fx scenario.Fixture) service.ResetResult {
	t.Helper()
	res, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "populate", Scenario: &fx})
	if err != nil {
		t.Fatalf("Reset(scenario): %v", err)
	}
	return res
}

func TestResetWithScenarioPopulatesExactData(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	teamBefore, _ := svc.Team(t.Context())
	fx := handFixture()

	res := resetTo(t, svc, fx)

	got, err := svc.Export(t.Context())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := handFixture()
	want.Team = &scenario.Team{ID: teamBefore.ID, Name: "Acme", Domain: "acme"} // the team id is stable across reset
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("exported state (-want +got):\n%s", diff)
	}
	if want := (service.ResetCounts{Users: 3, Channels: 4, Messages: 5}); res.Counts != want {
		t.Errorf("counts = %+v, want %+v", res.Counts, want)
	}
}

func TestScenarioComputesThreadCounts(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	resetTo(t, svc, handFixture())

	root, err := st.Message(t.Context(), "CGEN", "1700000100.000001")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if root.ReplyCount != 1 || root.LatestReply != "1700000200.000002" {
		t.Errorf("root reply_count=%d latest=%q, want 1 and the reply ts", root.ReplyCount, root.LatestReply)
	}
}

func TestExportWipeImportExportIsIdentity(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	ctx := t.Context()
	caller, bot := usersOf(t, svc)
	general := generalID(t, svc, caller)
	root, err := svc.PostMessage(ctx, caller, service.PostOpts{Channel: general, Content: service.Content{Text: "root", Blocks: json.RawMessage(`[{"type":"divider"}]`)}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := svc.PostMessage(ctx, bot, service.PostOpts{Channel: general, ThreadTS: root.TS, Content: service.Content{Text: "bot reply", Attachments: json.RawMessage(`[{"text":"a"}]`)}}); err != nil {
		t.Fatalf("PostMessage bot: %v", err)
	}
	for _, who := range []service.User{caller, bot} {
		if err := svc.AddReaction(ctx, who, general, root.TS, "eyes"); err != nil {
			t.Fatalf("AddReaction: %v", err)
		}
	}
	if _, err := svc.UpdateMessage(ctx, caller, service.UpdateOpts{Channel: general, TS: root.TS, Content: service.Content{Text: "root v2"}}); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	if _, err := svc.OpenDM(ctx, caller, []string{bot.ID}); err != nil {
		t.Fatalf("OpenDM: %v", err)
	}
	before, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if _, err := svc.Reset(ctx, service.ResetOpts{Seed: "wipe"}); err != nil { // wipe
		t.Fatalf("Reset: %v", err)
	}
	if _, err := svc.Restore(ctx, before); err != nil { // import
		t.Fatalf("Restore: %v", err)
	}

	after, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export again: %v", err)
	}
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("export -> wipe -> import -> export differs (-before +after):\n%s", diff)
	}
	rich := false
	for _, m := range after.Messages {
		rich = rich || (m.Edited != nil && len(m.Blocks) > 0 && len(m.Reactions) > 0)
	}
	if !rich {
		t.Error("test fixture lost its edited/blocks/reactions message; the identity check proves nothing")
	}
}

func TestScenarioValidationFailsWithPointersAndChangesNothing(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	before, _ := svc.Export(t.Context())
	fx := handFixture()
	fx.Users[0].ID = "X1"
	fx.Messages[1].ThreadTS = "9.000001"

	_, err := svc.Reset(t.Context(), service.ResetOpts{Scenario: &fx})

	var se *scenario.Error
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *scenario.Error", err)
	}
	pointers := []string{}
	for _, i := range se.Issues {
		pointers = append(pointers, i.Pointer)
	}
	for _, want := range []string{"/users/0/id", "/messages/1/thread_ts"} {
		if !slices.Contains(pointers, want) {
			t.Errorf("issues %v lack pointer %s", pointers, want)
		}
	}
	after, _ := svc.Export(t.Context())
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("failed reset changed state (-before +after):\n%s", diff)
	}
}

func TestScenarioMustNotCollideWithAppBotUsers(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	app := addApp(t, svc, st, "A1")
	fx := handFixture()
	fx.Users[0].ID = app.BotUserID

	_, err := svc.Reset(t.Context(), service.ResetOpts{Scenario: &fx})

	var se *scenario.Error
	if !errors.As(err, &se) || len(se.Issues) == 0 || se.Issues[0].Pointer != "/users/0/id" {
		t.Fatalf("err = %v, want a /users/0/id collision issue", err)
	}
}

func TestScenarioKeepsAppBotsAndLetsThemAuthorHistory(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	app := addApp(t, svc, st, "A2")
	fx := handFixture()
	fx.Messages = append(fx.Messages, scenario.Message{Channel: "CGEN", TS: "1700000700.000007", User: app.BotUserID, Text: "from the app"})

	resetTo(t, svc, fx)
	members, err := st.MembersOf(t.Context(), []string{"CGEN", "CRET", "GSEC"})
	if err != nil {
		t.Fatalf("MembersOf: %v", err)
	}
	if !slices.Contains(members["CGEN"], app.BotUserID) {
		t.Errorf("app bot not rejoined to #general: %v", members["CGEN"])
	}
	for _, id := range []string{"CRET", "GSEC"} {
		if slices.Contains(members[id], app.BotUserID) {
			t.Errorf("app bot joined non-joinable %s", id)
		}
	}
	m, err := st.Message(t.Context(), "CGEN", "1700000700.000007")
	if err != nil || m.UserID != app.BotUserID {
		t.Errorf("app-authored message = %+v, %v", m, err)
	}
	exported, err := svc.Export(t.Context())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, u := range exported.Users {
		if u.ID == app.BotUserID {
			t.Errorf("app bot user exported: %+v", u)
		}
	}
}

func TestScenarioAndRestoreEmitNoEvents(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	var n atomic.Int32
	svc.Subscribe(func(service.Event) { n.Add(1) })

	resetTo(t, svc, handFixture())
	fx, err := svc.Export(t.Context())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if _, err := svc.Restore(t.Context(), fx); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := n.Load(); got != 0 {
		t.Errorf("events = %d, want 0", got)
	}
}

func TestScenarioKeepsTsForwardOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ts   string
		// floor is a ts the next post must exceed.
		floor string
	}{
		{"old history", "100.000001", "100.000001"},
		{"future history", "9999999999.000001", "9999999999.000001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newSvc(t)
			fx := handFixture()
			fx.Messages = []scenario.Message{{Channel: "CGEN", TS: tt.ts, User: "UANN", Text: "x"}}
			resetTo(t, svc, fx)
			caller, _ := usersOf(t, svc)

			m, err := svc.PostMessage(t.Context(), caller, service.PostOpts{Channel: "CGEN", Content: service.Content{Text: "new"}})
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			if m.TS <= tt.floor && len(m.TS) == len(tt.floor) {
				t.Errorf("new ts %s <= fixture ts %s", m.TS, tt.floor)
			}
			if tt.name == "old history" && m.TS < "1000000000" {
				t.Errorf("new ts %s fell back to the fixture's old time", m.TS)
			}
		})
	}
}

func TestResetWithGenerate(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	gen := scenario.Fixture{Generate: &scenario.Generate{Users: 6, Channels: 4, MessagesPerChannel: 10, ThreadRatio: 0.3, PrivateChannels: 1}}

	a, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "g1", Scenario: &gen})
	if err != nil {
		t.Fatalf("Reset(generate): %v", err)
	}
	fa, _ := svc.Export(t.Context())
	if _, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "g1", Scenario: &gen}); err != nil {
		t.Fatalf("Reset again: %v", err)
	}
	fb, _ := svc.Export(t.Context())

	if want := (service.ResetCounts{Users: 7, Channels: 4, Messages: 40}); a.Counts != want {
		t.Errorf("counts = %+v, want %+v", a.Counts, want)
	}
	if diff := cmp.Diff(fa, fb); diff != "" {
		t.Errorf("same seed, different generated data (-a +b):\n%s", diff)
	}
	if _, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "g2", Scenario: &gen}); err != nil {
		t.Fatalf("Reset g2: %v", err)
	}
	if fc, _ := svc.Export(t.Context()); cmp.Equal(fa.Users, fc.Users) {
		t.Error("different seeds generated the same users")
	}
}

func TestResetGenerateRejectsBadParams(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	bad := scenario.Fixture{Generate: &scenario.Generate{Users: 99999}}
	_, err := svc.Reset(t.Context(), service.ResetOpts{Scenario: &bad})
	var se *scenario.Error
	if !errors.As(err, &se) || se.Issues[0].Pointer != "/generate/users" {
		t.Errorf("err = %v, want /generate/users issue", err)
	}
}

func TestSeedEmptyOnlyPopulatesAnEmptyDatabase(t *testing.T) {
	t.Parallel()

	st := storetest.NewEmptyStore(t)
	fx := handFixture()
	in := service.SeedInput{Fixture: &fx, Seed: "s", Now: storetest.PinnedNow()}

	did, err := service.SeedEmpty(t.Context(), st, in)
	if err != nil || !did {
		t.Fatalf("first SeedEmpty = %v, %v; want true", did, err)
	}
	other := handFixture()
	other.Users = other.Users[:1]
	other.Channels, other.DMs, other.Messages = nil, nil, nil
	did, err = service.SeedEmpty(t.Context(), st, service.SeedInput{Fixture: &other, Seed: "s", Now: storetest.PinnedNow()})
	if err != nil || did {
		t.Fatalf("second SeedEmpty = %v, %v; want false, nil", did, err)
	}
	svc, err := service.New(t.Context(), st, fixedClock(), "ids")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, _ := svc.Export(t.Context())
	if diff := cmp.Diff(fx, got); diff != "" {
		t.Errorf("seeded state (-want +got):\n%s", diff)
	}
}

func TestSeedEmptyDefaultsToTheDemoSeed(t *testing.T) {
	t.Parallel()

	st := storetest.NewEmptyStore(t)
	did, err := service.SeedEmpty(t.Context(), st, service.SeedInput{Seed: "demo", Now: storetest.PinnedNow()})
	if err != nil || !did {
		t.Fatalf("SeedEmpty = %v, %v", did, err)
	}
	users, _ := st.Users(t.Context())
	if len(users) < 2 {
		t.Errorf("users = %d, want the demo workspace", len(users))
	}
}

func TestPopulateScaleIsFastEnough(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	gen := scenario.Fixture{Generate: &scenario.Generate{Seed: "scale", Users: 400, Channels: 50, MessagesPerChannel: 40, ThreadRatio: 0.2, PrivateChannels: 5}}

	start := time.Now()
	res, err := svc.Reset(t.Context(), service.ResetOpts{Seed: "scale", Scenario: &gen})
	took := time.Since(start)

	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if want := (service.ResetCounts{Users: 401, Channels: 50, Messages: 2000}); res.Counts != want {
		t.Errorf("counts = %+v, want %+v", res.Counts, want)
	}
	// Target is well under 2s; the bound is loose so a loaded CI box never flakes (see BenchmarkPopulateScale).
	if took > 20*time.Second {
		t.Errorf("populate took %v, want well under 2s on a quiet machine", took)
	}
}

func BenchmarkPopulateScale(b *testing.B) {
	st := storetest.NewStore(b)
	svc, err := service.New(b.Context(), st, fixedClock(), "bench")
	if err != nil {
		b.Fatal(err)
	}
	gen := scenario.Fixture{Generate: &scenario.Generate{Seed: "scale", Users: 400, Channels: 50, MessagesPerChannel: 40, ThreadRatio: 0.2, PrivateChannels: 5}}
	b.ResetTimer()
	for range b.N {
		if _, err := svc.Reset(b.Context(), service.ResetOpts{Seed: "scale", Scenario: &gen}); err != nil {
			b.Fatal(err)
		}
	}
}
