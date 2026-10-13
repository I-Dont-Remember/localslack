package scenario_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
)

func TestDiffIdenticalIsEmptyAndJSONHasEmptyArrays(t *testing.T) {
	t.Parallel()

	d := scenario.Diff(validFixture(), validFixture())
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"users":{"added":[],"removed":[],"changed":[]},"conversations":{"added":[],"removed":[],"changed":[]},` +
		`"members":{"added":[],"removed":[],"changed":[]},"messages":{"added":[],"removed":[],"changed":[]},` +
		`"reactions":{"added":[],"removed":[],"changed":[]}}`
	if string(b) != want {
		t.Errorf("diff JSON = %s\nwant %s", b, want)
	}
}

func TestDiffReportsEachSection(t *testing.T) {
	t.Parallel()

	from := validFixture()
	to := validFixture()
	// users: one removed (U3 + its refs dropped is not needed for diff), one added, one changed.
	to.Users = []scenario.User{
		{ID: "U1", Name: "ann", RealName: "Annie"},
		{ID: "W2", Name: "bob"},
		{ID: "U9", Name: "zed"},
	}
	// conversations: G1 removed, C1 changed, C2 added.
	to.Channels = []scenario.Channel{
		{ID: "C1", Name: "lobby", Creator: "U1", Created: 10, IsArchived: true, Members: []string{"U1", "U9"}},
		{ID: "C2", Name: "new", Created: 12, Members: []string{"U1"}},
	}
	// messages: root edited, reply removed, bot message removed, one added; blocks changed on none.
	to.Messages = []scenario.Message{
		{
			Channel: "C1", TS: "100.000001", User: "U1", Text: "root v2", Edited: &scenario.Edit{User: "U1", TS: "200.000001"},
			Reactions: []scenario.Reaction{{Name: "eyes", Users: []string{"U1"}}},
		},
		{Channel: "C2", TS: "300.000001", User: "U1", Text: "fresh"},
		{Channel: "D1", TS: "101.000001", User: "U1", Text: "dm", Reactions: []scenario.Reaction{{Name: "+1", Users: []string{"W2"}}}},
	}
	// DMs unchanged.

	got := scenario.Diff(from, to)

	want := scenario.Delta{
		Users: scenario.Section[scenario.User]{
			Added:   []scenario.User{{ID: "U9", Name: "zed"}},
			Removed: []scenario.User{{ID: "U3", Name: "helper", IsBot: true, BotID: "B1"}},
			Changed: []scenario.Change{{ID: "U1", Fields: map[string]scenario.FieldChange{"real_name": {From: "Ann", To: "Annie"}}}},
		},
		Conversations: scenario.Section[scenario.Conversation]{
			Added:   []scenario.Conversation{{ID: "C2", Name: "new", Kind: "channel", Created: 12}},
			Removed: []scenario.Conversation{{ID: "G1", Name: "secret", Kind: "channel", Created: 11, IsPrivate: true}},
			Changed: []scenario.Change{{ID: "C1", Fields: map[string]scenario.FieldChange{
				"name": {From: "general", To: "lobby"}, "is_archived": {From: false, To: true},
			}}},
		},
		Members: scenario.Section[scenario.Member]{
			Added:   []scenario.Member{{Channel: "C1", User: "U9"}, {Channel: "C2", User: "U1"}},
			Removed: []scenario.Member{{Channel: "C1", User: "W2"}, {Channel: "G1", User: "U1"}},
			Changed: []scenario.Change{},
		},
		Messages: scenario.Section[scenario.Message]{
			Added: []scenario.Message{{Channel: "C2", TS: "300.000001", User: "U1", Text: "fresh"}},
			Removed: []scenario.Message{
				{Channel: "C1", TS: "100.000002", User: "W2", Text: "reply", ThreadTS: "100.000001"},
				{Channel: "C1", TS: "100.000003", User: "U3", BotID: "B1", Text: "bot", Blocks: json.RawMessage(`[{"type":"divider"}]`)},
			},
			Changed: []scenario.Change{{Channel: "C1", TS: "100.000001", Fields: map[string]scenario.FieldChange{
				"text":   {From: "root", To: "root v2"},
				"edited": {From: nil, To: map[string]any{"user": "U1", "ts": "200.000001"}},
			}}},
		},
		Reactions: scenario.Section[scenario.ReactionRef]{
			Added:   []scenario.ReactionRef{{Channel: "C1", TS: "100.000001", Name: "eyes", User: "U1"}},
			Removed: []scenario.ReactionRef{},
			Changed: []scenario.Change{},
		},
	}
	// DM D1 reaction is unchanged; the DM conversation and its members are unchanged too.
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Diff (-want +got):\n%s", diff)
	}
}

func TestDiffTreatsEquivalentJSONAsEqual(t *testing.T) {
	t.Parallel()

	a, b := validFixture(), validFixture()
	a.Messages[2].Blocks = json.RawMessage(`[{"type": "divider"}]`)
	b.Messages[2].Blocks = json.RawMessage(`[{"type":"divider"}]`)
	if d := scenario.Diff(a, b); len(d.Messages.Changed) != 0 {
		t.Errorf("whitespace-only blocks difference reported: %+v", d.Messages.Changed)
	}
}

func TestDiffOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	from := scenario.Fixture{Version: 1}
	to := validFixture()
	var first string
	for range 5 {
		b, _ := json.Marshal(scenario.Diff(from, to))
		if first == "" {
			first = string(b)
		} else if string(b) != first {
			t.Fatal("diff output differs between runs")
		}
	}
	d := scenario.Diff(from, to)
	if ids := userIDs(d.Users.Added); strings.Join(ids, ",") != "U1,U3,W2" {
		t.Errorf("added users order = %v, want sorted by id", ids)
	}
}

func userIDs(us []scenario.User) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}
