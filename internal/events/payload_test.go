package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
	"localslack/internal/service"
)

func routeNow() time.Time { return time.Unix(1_700_000_000, 123_000_000) }

func publicGen() service.ChannelInfo {
	return service.ChannelInfo{ID: "C1", Name: "general", Kind: service.KindChannel}
}

func allBots() []service.App {
	return []service.App{
		{ID: "AAAA", BotUserID: "UBOTA", BotID: "BBOTA", BotEvents: []string{"message.channels", "app_mention", "reaction_added"}},
		{ID: "ABBB", BotUserID: "UBOTB", BotID: "BBOTB", BotEvents: []string{"message.channels", "message.im"}},
		{ID: "ACCC", BotUserID: "UBOTC", BotID: "BBOTC", BotEvents: []string{"message.groups", "message.mpim"}},
		// no subscriptions: never receives anything.
		{ID: "ADDD", BotUserID: "UBOTD"},
	}
}

func scope(ch service.ChannelInfo, targets ...string) service.Scope {
	return service.Scope{Channel: ch, Targets: targets}
}

type routed struct {
	AppID string
	Type  string
	Event map[string]any
}

func summarize(rs []events.Routed) []routed {
	out := make([]routed, 0, len(rs))
	for _, r := range rs {
		out = append(out, routed{AppID: r.App.ID, Type: r.Type, Event: r.Event})
	}
	return out
}

func TestRouteMapping(t *testing.T) {
	t.Parallel()

	human := service.Message{ChannelID: "C1", TS: "100.000001", UserID: "UHUMAN", Text: "hello there"}
	mention := service.Message{ChannelID: "C1", TS: "100.000002", UserID: "UHUMAN", Text: "hey <@UBOTA> ping"}
	botPost := service.Message{ChannelID: "C1", TS: "100.000003", UserID: "UBOTA", BotID: "BBOTA", Text: "pong <@UBOTA>"}
	threaded := service.Message{ChannelID: "C1", TS: "100.000004", UserID: "UHUMAN", Text: "reply", ThreadTS: "100.000001"}
	privateCh := service.ChannelInfo{ID: "G1", Name: "secret", Kind: service.KindChannel, IsPrivate: true}
	im := service.ChannelInfo{ID: "D1", Kind: service.KindIM}
	mpim := service.ChannelInfo{ID: "M1", Kind: service.KindMPIM}

	chanMsg := func(m service.Message, channelType string) map[string]any {
		e := map[string]any{
			"type": "message", "channel": m.ChannelID, "user": m.UserID, "text": m.Text, "ts": m.TS,
			"event_ts": m.TS, "channel_type": channelType, "team": "T1",
		}
		if m.ThreadTS != "" {
			e["thread_ts"] = m.ThreadTS
		}
		if m.BotID != "" {
			e["bot_id"] = m.BotID
		}
		return e
	}
	mentionEv := func(m service.Message) map[string]any {
		e := map[string]any{
			"type": "app_mention", "channel": m.ChannelID, "user": m.UserID, "text": m.Text, "ts": m.TS,
			"event_ts": m.TS, "team": "T1",
		}
		return e
	}

	tests := []struct {
		name string
		ev   service.Event
		want []routed
	}{
		{
			name: "public message reaches subscribed member bots only",
			ev:   service.MessagePosted{Msg: human, Scope: scope(publicGen(), "UBOTA", "UBOTB", "UBOTC", "UBOTD")},
			want: []routed{
				{"AAAA", "message", chanMsg(human, "channel")},
				{"ABBB", "message", chanMsg(human, "channel")},
			},
		},
		{
			name: "non-member bot gets nothing",
			ev:   service.MessagePosted{Msg: human, Scope: scope(publicGen(), "UBOTB")},
			want: []routed{{"ABBB", "message", chanMsg(human, "channel")}},
		},
		{
			name: "mention adds app_mention for subscribed mentioned bot even if not a member",
			ev:   service.MessagePosted{Msg: mention, Scope: scope(publicGen(), "UBOTB")},
			want: []routed{
				{"AAAA", "app_mention", mentionEv(mention)},
				{"ABBB", "message", chanMsg(mention, "channel")},
			},
		},
		{
			name: "member mentioned bot gets message and app_mention",
			ev:   service.MessagePosted{Msg: mention, Scope: scope(publicGen(), "UBOTA")},
			want: []routed{
				{"AAAA", "message", chanMsg(mention, "channel")},
				{"AAAA", "app_mention", mentionEv(mention)},
			},
		},
		{
			name: "bot own post is delivered but never mentions itself",
			ev:   service.MessagePosted{Msg: botPost, Scope: scope(publicGen(), "UBOTA")},
			want: []routed{{"AAAA", "message", chanMsg(botPost, "channel")}},
		},
		{
			name: "thread reply carries thread_ts",
			ev:   service.MessagePosted{Msg: threaded, Scope: scope(publicGen(), "UBOTB")},
			want: []routed{{"ABBB", "message", chanMsg(threaded, "channel")}},
		},
		{
			name: "private channel uses message.groups",
			ev:   service.MessagePosted{Msg: human, Scope: scope(privateCh, "UBOTA", "UBOTC")},
			want: []routed{{"ACCC", "message", chanMsg(human, "group")}},
		},
		{
			name: "im uses message.im",
			ev:   service.MessagePosted{Msg: human, Scope: scope(im, "UBOTA", "UBOTB")},
			want: []routed{{"ABBB", "message", chanMsg(human, "im")}},
		},
		{
			name: "mpim uses message.mpim",
			ev:   service.MessagePosted{Msg: human, Scope: scope(mpim, "UBOTC")},
			want: []routed{{"ACCC", "message", chanMsg(human, "mpim")}},
		},
		{
			name: "reaction_added to member bots that subscribe",
			ev:   service.ReactionAdded{User: "UHUMAN", Reaction: "eyes", Msg: human, Scope: scope(publicGen(), "UBOTA", "UBOTB")},
			want: []routed{{"AAAA", "reaction_added", map[string]any{
				"type": "reaction_added", "user": "UHUMAN", "reaction": "eyes", "item_user": "UHUMAN",
				"item":     map[string]any{"type": "message", "channel": "C1", "ts": "100.000001"},
				"event_ts": "1700000000.123000",
			}}},
		},
		{
			name: "message_changed carries previous_message and edited",
			ev: service.MessageChanged{
				Prev:  human,
				Msg:   service.Message{ChannelID: "C1", TS: "100.000001", UserID: "UHUMAN", Text: "edited", Edited: &service.Edit{User: "UHUMAN", TS: "200.000001"}},
				Scope: scope(publicGen(), "UBOTB"),
			},
			want: []routed{{"ABBB", "message", map[string]any{
				"type": "message", "subtype": "message_changed", "hidden": true, "channel": "C1", "channel_type": "channel",
				"ts": "200.000001", "event_ts": "200.000001",
				"message": map[string]any{
					"type": "message", "user": "UHUMAN", "text": "edited", "ts": "100.000001", "team": "T1",
					"edited": map[string]any{"user": "UHUMAN", "ts": "200.000001"},
				},
				"previous_message": map[string]any{"type": "message", "user": "UHUMAN", "text": "hello there", "ts": "100.000001", "team": "T1"},
			}}},
		},
		{
			name: "message_deleted",
			ev:   service.MessageDeleted{Msg: human, Scope: scope(publicGen(), "UBOTB")},
			want: []routed{{"ABBB", "message", map[string]any{
				"type": "message", "subtype": "message_deleted", "hidden": true, "channel": "C1", "channel_type": "channel",
				"ts": "1700000000.123000", "event_ts": "1700000000.123000", "deleted_ts": "100.000001",
				"previous_message": map[string]any{"type": "message", "user": "UHUMAN", "text": "hello there", "ts": "100.000001", "team": "T1"},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := summarize(events.Route(tt.ev, allBots(), "T1", routeNow()))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Route (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnvelope(t *testing.T) {
	t.Parallel()

	app := service.App{ID: "AAAA", BotUserID: "UBOTA", VerificationToken: "vt"}
	body, err := events.Envelope(app, "T1", "Ev123", routeNow(), map[string]any{"type": "app_mention", "channel": "C1"})
	if err != nil {
		t.Fatalf("Envelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	ctx, _ := got["event_context"].(string)
	if len(ctx) < 3 || ctx[:2] != "4-" {
		t.Errorf("event_context = %q, want 4-<opaque>", ctx)
	}
	delete(got, "event_context")
	want := map[string]any{
		"token": "vt", "team_id": "T1", "api_app_id": "AAAA", "type": "event_callback", "event_id": "Ev123",
		"event_time": float64(1_700_000_000), "is_ext_shared_channel": false,
		"event": map[string]any{"type": "app_mention", "channel": "C1"},
		"authorizations": []any{map[string]any{
			"enterprise_id": nil, "team_id": "T1", "user_id": "UBOTA", "is_bot": true, "is_enterprise_install": false,
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("envelope (-want +got):\n%s", diff)
	}
}

func TestEnvelopeMerge(t *testing.T) {
	t.Parallel()

	base := []byte(`{"a":1,"nested":{"x":1},"drop":true}`)
	got, err := events.MergeEnvelope(base, map[string]any{"a": 2, "nested": map[string]any{"y": 2}, "drop": nil})
	if err != nil {
		t.Fatalf("MergeEnvelope: %v", err)
	}
	if want := `{"a":2,"nested":{"y":2}}`; string(got) != want {
		t.Errorf("merged = %s, want %s (shallow merge, null deletes)", got, want)
	}
}
