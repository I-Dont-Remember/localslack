package scenario_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
)

func TestFixtureJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := scenario.Fixture{
		Version: scenario.Version,
		Team:    &scenario.Team{ID: "T1", Name: "Acme", Domain: "acme"},
		Users:   []scenario.User{{ID: "U1", Name: "ann"}, {ID: "U2", Name: "bot", IsBot: true, BotID: "B1"}},
		Channels: []scenario.Channel{
			{ID: "C1", Name: "general", Members: []string{"U1", "U2"}},
			{ID: "G1", Name: "secret", IsPrivate: true, Members: []string{"U1"}},
		},
		DMs: []scenario.DM{{ID: "D1", Members: []string{"U1", "U2"}}},
		Messages: []scenario.Message{{
			Channel: "C1", TS: "1.000001", User: "U2", Text: "hi", BotID: "B1",
			Blocks:    json.RawMessage(`[{"type":"divider"}]`),
			Edited:    &scenario.Edit{User: "U2", TS: "2.000001"},
			Reactions: []scenario.Reaction{{Name: "eyes", Users: []string{"U1"}}},
		}},
		Generate: &scenario.Generate{Seed: "s", Users: 3, Channels: 2, MessagesPerChannel: 5, ThreadRatio: 0.25, PrivateChannels: 1},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got scenario.Fixture
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
}
