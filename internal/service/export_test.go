package service_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
	"localslack/internal/service"
	"localslack/internal/store"
)

func TestExportCoversEverythingButAppBotUsers(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	ctx := t.Context()
	app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed(t, "Exported", "")})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	team, _ := svc.Team(ctx)
	caller, defaultBot := usersOf(t, svc)
	err = st.Write(ctx, func(tx *store.Tx) error {
		steps := []error{
			tx.InsertConversation(ctx, store.Conversation{ID: "G1", TeamID: team.ID, Name: "vault", Kind: store.KindChannel, Creator: caller.ID, Created: 42, IsPrivate: true, IsArchived: true}),
			tx.AddMember(ctx, "G1", caller.ID),
			tx.AddMember(ctx, "G1", app.BotUserID),
			tx.InsertMessage(ctx, store.Message{ChannelID: "G1", TS: "9000.000001", UserID: caller.ID, Text: "root"}),
			tx.InsertMessage(ctx, store.Message{
				ChannelID: "G1", TS: "9000.000002", UserID: defaultBot.ID, Text: "rich", ThreadTS: "9000.000001",
				BotID: defaultBot.BotID, Subtype: "bot_message",
				Blocks:      json.RawMessage(`[{"type":"divider"}]`),
				Attachments: json.RawMessage(`[{"text":"att"}]`),
				Metadata:    json.RawMessage(`{"event_type":"x","event_payload":{"k":1}}`),
				Edited:      &store.Edit{User: defaultBot.ID, TS: "9001.000001"},
			}),
			tx.InsertReaction(ctx, store.Reaction{ChannelID: "G1", TS: "9000.000001", Name: "eyes", UserID: caller.ID, Created: 1}),
			tx.InsertReaction(ctx, store.Reaction{ChannelID: "G1", TS: "9000.000001", Name: "tada", UserID: defaultBot.ID, Created: 2}),
			tx.InsertReaction(ctx, store.Reaction{ChannelID: "G1", TS: "9000.000001", Name: "eyes", UserID: defaultBot.ID, Created: 3}),
		}
		for _, e := range steps {
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if got.Version != scenario.Version || got.Generate != nil {
		t.Errorf("version/generate = %d / %v", got.Version, got.Generate)
	}
	if want := (&scenario.Team{ID: team.ID, Name: team.Name, Domain: team.Domain}); !cmp.Equal(got.Team, want) {
		t.Errorf("team = %+v, want %+v", got.Team, want)
	}
	for _, u := range got.Users {
		if u.ID == app.BotUserID {
			t.Errorf("app bot user %s must not be exported", u.ID)
		}
	}
	wantBot := scenario.User{ID: defaultBot.ID, Name: defaultBot.Name, RealName: defaultBot.RealName, IsBot: true, BotID: defaultBot.BotID}
	if i := indexFunc(got.Users, func(u scenario.User) bool { return u.ID == defaultBot.ID }); i < 0 || got.Users[i] != wantBot {
		t.Errorf("default bot user missing or wrong in %+v, want %+v", got.Users, wantBot)
	}

	i := indexFunc(got.Channels, func(c scenario.Channel) bool { return c.ID == "G1" })
	if i < 0 {
		t.Fatalf("channel G1 missing in %+v", got.Channels)
	}
	wantCh := scenario.Channel{ID: "G1", Name: "vault", Creator: caller.ID, Created: 42, IsArchived: true, IsPrivate: true, Members: slices.Sorted(slices.Values([]string{caller.ID, app.BotUserID}))}
	if diff := cmp.Diff(wantCh, got.Channels[i]); diff != "" {
		t.Errorf("channel G1 (-want +got):\n%s", diff)
	}

	for _, c := range got.Channels {
		if !c.IsPrivate && !c.IsArchived && slices.Contains(c.Members, app.BotUserID) {
			t.Errorf("public channel %s exports app bot %s; restore re-joins it", c.Name, app.BotUserID)
		}
	}

	var msgs []scenario.Message
	for _, m := range got.Messages {
		if m.Channel == "G1" {
			msgs = append(msgs, m)
		}
	}
	wantMsgs := []scenario.Message{
		{Channel: "G1", TS: "9000.000001", User: caller.ID, Text: "root", Reactions: []scenario.Reaction{
			{Name: "eyes", Users: []string{caller.ID, defaultBot.ID}}, {Name: "tada", Users: []string{defaultBot.ID}},
		}},
		{
			Channel: "G1", TS: "9000.000002", User: defaultBot.ID, Text: "rich", ThreadTS: "9000.000001",
			BotID: defaultBot.BotID, Subtype: "bot_message",
			Blocks:      json.RawMessage(`[{"type":"divider"}]`),
			Attachments: json.RawMessage(`[{"text":"att"}]`),
			Metadata:    json.RawMessage(`{"event_type":"x","event_payload":{"k":1}}`),
			Edited:      &scenario.Edit{User: defaultBot.ID, TS: "9001.000001"},
		},
	}
	if diff := cmp.Diff(wantMsgs, msgs); diff != "" {
		t.Errorf("messages in G1 (-want +got):\n%s", diff)
	}
}

func TestExportSplitsChannelsFromDMs(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	convs, err := st.Conversations(t.Context())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	var wantCh, wantDM int
	for _, c := range convs {
		if c.Kind == service.KindChannel {
			wantCh++
		} else {
			wantDM++
		}
	}

	got, err := svc.Export(t.Context())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(got.Channels) != wantCh || len(got.DMs) != wantDM || wantCh == 0 {
		t.Errorf("channels/dms = %d/%d, want %d/%d", len(got.Channels), len(got.DMs), wantCh, wantDM)
	}
}

// TestExportIsOneConsistentSnapshot hammers the workspace with multi-table
// writes while exporting: every export must contain whole transactions only.
func TestExportIsOneConsistentSnapshot(t *testing.T) {
	t.Parallel()

	svc, st := newSvc(t)
	ctx := t.Context()
	caller, _ := usersOf(t, svc)
	general := generalID(t, svc, caller)
	team, _ := svc.Team(ctx)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			id, ts := "UCONS"+strconv.Itoa(i), "8000."+strconv.Itoa(100000+i)
			err := st.Write(ctx, func(tx *store.Tx) error {
				for _, e := range []error{
					tx.InsertUser(ctx, store.User{ID: id, TeamID: team.ID, Name: id}),
					tx.AddMember(ctx, general, id),
					tx.InsertMessage(ctx, store.Message{ChannelID: general, TS: ts, UserID: id, Text: "c"}),
					tx.InsertReaction(ctx, store.Reaction{ChannelID: general, TS: ts, Name: "eyes", UserID: id, Created: int64(i)}),
				} {
					if e != nil {
						return e
					}
				}
				return nil
			})
			if err != nil {
				t.Errorf("writer: %v", err)
				return
			}
		}
	}()

	for range 60 {
		fx, err := svc.Export(ctx)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		if problem := dangling(fx); problem != "" {
			close(stop)
			wg.Wait()
			t.Fatalf("inconsistent export: %s", problem)
		}
	}
	close(stop)
	wg.Wait()
}

// dangling returns a description of the first reference in fx that points nowhere.
func dangling(fx scenario.Fixture) string {
	users := map[string]bool{}
	for _, u := range fx.Users {
		users[u.ID] = true
	}
	for _, c := range fx.Channels {
		for _, u := range c.Members {
			if !users[u] && len(u) > 5 && u[:5] == "UCONS" {
				return "member " + u + " of " + c.ID + " is not an exported user"
			}
		}
	}
	for _, m := range fx.Messages {
		if len(m.User) > 5 && m.User[:5] == "UCONS" && !users[m.User] {
			return "message " + m.TS + " by missing user " + m.User
		}
		if len(m.User) > 5 && m.User[:5] == "UCONS" && !hasReaction(m, "eyes", m.User) {
			return "message " + m.TS + " lacks the reaction written in the same transaction"
		}
	}
	return ""
}

func hasReaction(m scenario.Message, name, user string) bool {
	for _, r := range m.Reactions {
		if r.Name == name && slices.Contains(r.Users, user) {
			return true
		}
	}
	return false
}

func indexFunc[T any](s []T, f func(T) bool) int {
	for i, v := range s {
		if f(v) {
			return i
		}
	}
	return -1
}
