package scenario_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/ids"
	"localslack/internal/scenario"
)

func TestResolveFillsIDsAndNames(t *testing.T) {
	t.Parallel()

	in := scenario.Fixture{
		Version: 1,
		Team:    &scenario.Team{Name: "Acme", Domain: "acme"},
		Users:   []scenario.User{{Name: "ann"}, {ID: "U2", Name: "bob"}, {Name: "bot", IsBot: true}},
		Channels: []scenario.Channel{
			{Name: "general", Creator: "ann", Members: []string{"ann", "U2"}},
			{Name: "vault", IsPrivate: true, Members: []string{"bob"}},
		},
		DMs: []scenario.DM{{Members: []string{"ann", "bob"}}, {Members: []string{"ann", "bob", "bot"}}},
		Messages: []scenario.Message{
			{Channel: "general", TS: "100.000001", User: "ann", Text: "root", Reactions: []scenario.Reaction{{Name: "eyes", Users: []string{"bob"}}}},
			{Channel: "#general", TS: "100.000002", User: "bob", ThreadTS: "100.000001", Edited: &scenario.Edit{User: "bob", TS: "101.000001"}},
			{Channel: "vault", TS: "100.000003", User: "@ann"},
			{Channel: "100.000003", TS: "1.000001", User: "nobody"},
		},
	}
	now := time.Unix(5000, 0)

	got := scenario.Resolve(in, ids.NewGenerator("r"), now)

	if again := scenario.Resolve(in, ids.NewGenerator("r"), now); !cmp.Equal(got, again) {
		t.Error("Resolve is not deterministic for the same id source")
	}
	if in.Users[0].ID != "" || in.Channels[0].ID != "" || in.Messages[0].Channel != "general" {
		t.Error("Resolve mutated its input")
	}
	if got.Team.ID == "" || got.Team.ID[0] != 'T' {
		t.Errorf("team id = %q, want T...", got.Team.ID)
	}
	ann, bob, bot := got.Users[0], got.Users[1], got.Users[2]
	if ann.ID[0] != 'U' || bob.ID != "U2" || bot.BotID == "" || bot.BotID[0] != 'B' {
		t.Errorf("users = %+v", got.Users)
	}
	general, vault := got.Channels[0], got.Channels[1]
	if general.ID[0] != 'C' || vault.ID[0] != 'G' || general.Created != 5000 {
		t.Errorf("channels = %+v", got.Channels)
	}
	if general.Creator != ann.ID || !cmp.Equal(general.Members, []string{ann.ID, "U2"}) || !cmp.Equal(vault.Members, []string{"U2"}) {
		t.Errorf("channel refs not resolved: %+v", got.Channels)
	}
	if got.DMs[0].ID[0] != 'D' || got.DMs[1].ID[0] != 'G' || !cmp.Equal(got.DMs[1].Members, []string{ann.ID, "U2", bot.ID}) {
		t.Errorf("dms = %+v", got.DMs)
	}
	wantMsgs := []struct{ channel, user string }{{general.ID, ann.ID}, {general.ID, "U2"}, {vault.ID, ann.ID}, {"100.000003", "nobody"}}
	for i, w := range wantMsgs {
		if m := got.Messages[i]; m.Channel != w.channel || m.User != w.user {
			t.Errorf("message %d = %s/%s, want %s/%s", i, m.Channel, m.User, w.channel, w.user)
		}
	}
	if r := got.Messages[0].Reactions[0].Users[0]; r != "U2" {
		t.Errorf("reaction user = %q, want U2", r)
	}
	if e := got.Messages[1].Edited; e.User != "U2" {
		t.Errorf("edited user = %q, want U2", e.User)
	}
	if issues := scenario.Validate(withoutLast(got), scenario.Reserved{}); len(issues) != 0 {
		t.Errorf("resolved fixture invalid: %+v", issues)
	}
}

func withoutLast(fx scenario.Fixture) scenario.Fixture {
	fx.Messages = fx.Messages[:len(fx.Messages)-1]
	return fx
}

func TestResolveNormalisesRootThreadTS(t *testing.T) {
	t.Parallel()

	in := scenario.Fixture{Messages: []scenario.Message{{Channel: "C1", TS: "1.000001", ThreadTS: "1.000001"}}}
	got := scenario.Resolve(in, ids.NewGenerator("r"), time.Unix(1, 0))
	if got.Messages[0].ThreadTS != "" {
		t.Errorf("thread_ts = %q, want a root to carry none", got.Messages[0].ThreadTS)
	}
}

func TestResolveLeavesGivenIDsAlone(t *testing.T) {
	t.Parallel()

	in := validFixture()
	if got := scenario.Resolve(in, ids.NewGenerator("r"), time.Unix(9, 0)); !cmp.Equal(in, got) {
		t.Errorf("Resolve changed a complete fixture:\n%s", cmp.Diff(in, got))
	}
}
