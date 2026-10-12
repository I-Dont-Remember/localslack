package scenario_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
)

func validFixture() scenario.Fixture {
	return scenario.Fixture{
		Version: 1,
		Team:    &scenario.Team{ID: "T1", Name: "Acme", Domain: "acme"},
		Users: []scenario.User{
			{ID: "U1", Name: "ann", RealName: "Ann"},
			{ID: "W2", Name: "bob"},
			{ID: "U3", Name: "helper", IsBot: true, BotID: "B1"},
		},
		Channels: []scenario.Channel{
			{ID: "C1", Name: "general", Creator: "U1", Created: 10, Members: []string{"U1", "W2"}},
			{ID: "G1", Name: "secret", Created: 11, IsPrivate: true, Members: []string{"U1"}},
		},
		DMs: []scenario.DM{{ID: "D1", Creator: "U1", Created: 12, Members: []string{"U1", "W2"}}, {ID: "G2", Creator: "U1", Created: 12, Members: []string{"U1", "W2", "U3"}}},
		Messages: []scenario.Message{
			{Channel: "C1", TS: "100.000001", User: "U1", Text: "root"},
			{Channel: "C1", TS: "100.000002", User: "W2", Text: "reply", ThreadTS: "100.000001"},
			{Channel: "C1", TS: "100.000003", User: "U3", BotID: "B1", Text: "bot", Blocks: json.RawMessage(`[{"type":"divider"}]`)},
			{Channel: "D1", TS: "101.000001", User: "U1", Text: "dm", Reactions: []scenario.Reaction{{Name: "+1", Users: []string{"W2"}}}},
		},
	}
}

func is(pointer, message string) scenario.Issue {
	return scenario.Issue{Pointer: pointer, Message: message}
}

func TestValidateAcceptsAValidFixture(t *testing.T) {
	t.Parallel()

	if got := scenario.Validate(validFixture(), scenario.Reserved{}); len(got) != 0 {
		t.Errorf("Validate = %+v, want none", got)
	}
}

func TestValidateReportsEveryProblemWithPointers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*scenario.Fixture)
		res    scenario.Reserved
		want   []scenario.Issue
	}{
		{
			name:   "version",
			mutate: func(f *scenario.Fixture) { f.Version = 2 },
			want:   []scenario.Issue{is("/version", "unsupported version 2; want 1")},
		},
		{
			name:   "team id prefix",
			mutate: func(f *scenario.Fixture) { f.Team.ID = "X1" },
			want:   []scenario.Issue{is("/team/id", `id "X1" must start with T`)},
		},
		{
			name:   "user id prefix",
			mutate: func(f *scenario.Fixture) { f.Users = append(f.Users, scenario.User{ID: "C9", Name: "zed"}) },
			want:   []scenario.Issue{is("/users/3/id", `id "C9" must start with U or W`)},
		},
		{
			name:   "duplicate user id and name",
			mutate: func(f *scenario.Fixture) { f.Users = append(f.Users, scenario.User{ID: "U1", Name: "ann"}) },
			want: []scenario.Issue{
				is("/users/3/id", `duplicate id "U1" (also /users/0/id)`),
				is("/users/3/name", `duplicate name "ann" (also /users/0/name)`),
			},
		},
		{
			name:   "bot rules",
			mutate: func(f *scenario.Fixture) { f.Users[0].BotID = "B7"; f.Users[2].BotID = "" },
			want: []scenario.Issue{
				is("/users/0/bot_id", "bot_id is only for is_bot users"),
				is("/users/2/bot_id", "is_bot users need a bot_id"),
				is("/messages/2/bot_id", `bot_id "B1" does not match author bot_id ""`),
			},
		},
		{
			name: "bot id prefix and duplicate",
			mutate: func(f *scenario.Fixture) {
				f.Users = append(f.Users, scenario.User{ID: "U4", Name: "b2", IsBot: true, BotID: "B1"})
			},
			want: []scenario.Issue{is("/users/3/bot_id", `duplicate bot_id "B1" (also /users/2/bot_id)`)},
		},
		{
			name:   "message bot_id on human",
			mutate: func(f *scenario.Fixture) { f.Messages[0].BotID = "B1" },
			want:   []scenario.Issue{is("/messages/0/bot_id", "bot_id set but author U1 is not a bot")},
		},
		{
			name:   "non example email",
			mutate: func(f *scenario.Fixture) { f.Users[0].RealName = "ann@corp.com"; f.Users[1].Name = "bob@example.com" },
			want: []scenario.Issue{
				is("/users/0/real_name", `email "ann@corp.com" is not synthetic; use @example.com`),
				is("/users/1/name", "name must be lowercase letters, digits, '.', '_' or '-'"),
			},
		},
		{
			name:   "channel problems",
			mutate: func(f *scenario.Fixture) { f.Channels[1].Name = "General"; f.Channels[1].ID = "C1" },
			want: []scenario.Issue{
				is("/channels/1/id", `duplicate id "C1" (also /channels/0/id)`),
				is("/channels/1/name", "name must be lowercase letters, digits, '-' or '_' (max 80)"),
				is("/channels/1/name", `duplicate name "General" (also /channels/0/name)`),
			},
		},
		{
			name:   "duplicate channel name case-insensitive",
			mutate: func(f *scenario.Fixture) { f.Channels[1].Name = "general" },
			want:   []scenario.Issue{is("/channels/1/name", `duplicate name "general" (also /channels/0/name)`)},
		},
		{
			name: "unknown and duplicate members",
			mutate: func(f *scenario.Fixture) {
				f.Channels[0].Members = []string{"U1", "U9", "U1"}
				f.Channels[0].Creator = "U8"
			},
			want: []scenario.Issue{
				is("/channels/0/creator", `unknown user "U8"`),
				is("/channels/0/members/1", `unknown user "U9"`),
				is("/channels/0/members/2", `duplicate member "U1"`),
			},
		},
		{
			name: "dm id by size",
			mutate: func(f *scenario.Fixture) {
				f.DMs[1].ID = "D7"
				f.DMs = append(f.DMs, scenario.DM{ID: "G7", Members: []string{"W2", "U3"}})
			},
			want: []scenario.Issue{
				is("/dms/1/id", `id "D7" must start with G (3 or more members)`),
				is("/dms/2/id", `id "G7" must start with D (2 or fewer members)`),
			},
		},
		{
			name: "empty dm and duplicate member set",
			mutate: func(f *scenario.Fixture) {
				f.DMs = append(f.DMs, scenario.DM{ID: "D2", Members: []string{"W2", "U1"}}, scenario.DM{ID: "D3"})
			},
			want: []scenario.Issue{
				is("/dms/2/members", "same members as /dms/0"),
				is("/dms/3/members", "need at least one member"),
			},
		},
		{
			name: "message problems",
			mutate: func(f *scenario.Fixture) {
				f.Messages[0].TS = "100"
				f.Messages[0].Channel = "C9"
				f.Messages[0].User = ""
			},
			want: []scenario.Issue{
				is("/messages/0/channel", `unknown conversation "C9"`),
				is("/messages/0/ts", `ts "100" must look like 1700000000.000100`),
				is("/messages/0/user", "user is required"),
				is("/messages/1/thread_ts", `thread_ts "100.000001" is not a root message in this channel`),
			},
		},
		{
			name:   "duplicate message key",
			mutate: func(f *scenario.Fixture) { f.Messages[1].TS = "100.000001"; f.Messages[1].ThreadTS = "" },
			want:   []scenario.Issue{is("/messages/1/ts", `duplicate message C1/100.000001 (also /messages/0)`)},
		},
		{
			name: "thread root must be a root in the same channel",
			mutate: func(f *scenario.Fixture) {
				f.Messages[2].ThreadTS = "100.000002"
				f.Messages[3].ThreadTS = "100.000001"
			},
			want: []scenario.Issue{
				is("/messages/2/thread_ts", `thread_ts "100.000002" is not a root message in this channel`),
				is("/messages/3/thread_ts", `thread_ts "100.000001" is not a root message in this channel`),
			},
		},
		{
			name:   "reply must follow its root",
			mutate: func(f *scenario.Fixture) { f.Messages[1].TS = "099.000001" },
			want:   []scenario.Issue{is("/messages/1/thread_ts", "reply ts must be later than its thread_ts")},
		},
		{
			name: "reactions and edited",
			mutate: func(f *scenario.Fixture) {
				f.Messages[0].Reactions = []scenario.Reaction{{Name: "Bad Name", Users: []string{"U1", "U1"}}, {Name: "ok", Users: []string{"U2"}}, {Name: "ok"}}
				f.Messages[0].Edited = &scenario.Edit{User: "U7", TS: "x"}
			},
			want: []scenario.Issue{
				is("/messages/0/edited/user", `unknown user "U7"`),
				is("/messages/0/edited/ts", `ts "x" must look like 1700000000.000100`),
				is("/messages/0/reactions/0/name", "reaction name must be lowercase letters, digits, '_', '+' or '-'"),
				is("/messages/0/reactions/0/users/1", `duplicate user "U1"`),
				is("/messages/0/reactions/1/users/0", `unknown user "U2"`),
				is("/messages/0/reactions/2/name", `duplicate reaction "ok" (also /messages/0/reactions/1)`),
				is("/messages/0/reactions/2/users", "need at least one user"),
			},
		},
		{
			name: "content shapes",
			mutate: func(f *scenario.Fixture) {
				f.Messages[0].Blocks = json.RawMessage(`{"a":1}`)
				f.Messages[0].Attachments = json.RawMessage(`"x"`)
				f.Messages[0].Metadata = json.RawMessage(`[1]`)
			},
			want: []scenario.Issue{
				is("/messages/0/blocks", "blocks must be a JSON array"),
				is("/messages/0/attachments", "attachments must be a JSON array"),
				is("/messages/0/metadata", "metadata must be a JSON object"),
			},
		},
		{
			name:   "generate cannot mix with data",
			mutate: func(f *scenario.Fixture) { f.Generate = &scenario.Generate{Users: 2} },
			want:   []scenario.Issue{is("/generate", "generate cannot be combined with users, channels, dms or messages")},
		},
		{
			name:   "collisions with app bot users",
			res:    scenario.Reserved{UserIDs: []string{"U3"}, BotIDs: []string{"B1"}, UserNames: []string{"bob"}},
			mutate: func(*scenario.Fixture) {},
			want: []scenario.Issue{
				is("/users/1/name", `name "bob" belongs to an app bot user`),
				is("/users/2/id", `id "U3" belongs to an app bot user`),
				is("/users/2/bot_id", `bot_id "B1" belongs to an app bot user`),
			},
		},
		{
			name: "app bots may author, react and join dms but not join public channels",
			res:  scenario.Reserved{UserIDs: []string{"UAPP"}},
			mutate: func(f *scenario.Fixture) {
				f.Messages[0].User = "UAPP"
				f.Messages[3].Reactions = []scenario.Reaction{{Name: "eyes", Users: []string{"UAPP"}}}
				f.DMs[0].Members = []string{"U1", "UAPP"}
				f.Channels[1].Members = []string{"U1", "UAPP"}
				f.Channels[0].Members = []string{"U1", "UAPP"}
			},
			want: []scenario.Issue{is("/channels/0/members/1", `app bot "UAPP" is joined to public channels automatically; remove it`)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := validFixture()
			tt.mutate(&fx)
			got := scenario.Validate(fx, tt.res)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("issues (-want +got):\n%s", diff)
			}
		})
	}
}
