package events

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"localslack/internal/service"
)

// Routed is one inner event addressed to one app.
type Routed struct {
	App service.App
	// Type is the inner event type: message, app_mention or reaction_added.
	Type  string
	Event map[string]any
}

// Route maps a service event to the inner events each app should receive:
// subscription match plus the app's bot being a member of the channel
// (Scope.Targets), except app_mention in a public channel, which needs only the
// mention (private channels, DMs and group DMs still need membership). A bot's own
// posts are delivered. team is the workspace id; now stamps events that carry no ts.
func Route(ev service.Event, apps []service.App, team string, now time.Time) []Routed {
	var out []Routed
	for _, app := range apps {
		if app.BotUserID == "" {
			continue
		}
		subs := app.BotEvents
		for _, r := range routeOne(ev, app, team, now) {
			if r.needsMember && !slices.Contains(r.targets, app.BotUserID) {
				continue
			}
			if slices.Contains(subs, r.sub) {
				out = append(out, Routed{App: app, Type: r.typ, Event: r.event})
			}
		}
	}
	return out
}

// candidate is an event an app could get, before subscription/membership filters.
type candidate struct {
	typ, sub    string
	event       map[string]any
	needsMember bool
	targets     []string
}

func routeOne(ev service.Event, app service.App, team string, now time.Time) []candidate {
	switch e := ev.(type) {
	case service.MessagePosted:
		return postedCandidates(e, app, team)
	case service.MessageChanged:
		return []candidate{messageCandidate(e.Scope, changedEvent(e, team, now))}
	case service.MessageDeleted:
		return []candidate{messageCandidate(e.Scope, deletedEvent(e, team, now))}
	case service.ReactionAdded:
		return []candidate{{
			typ: "reaction_added", sub: "reaction_added", needsMember: true, targets: e.Targets,
			event: map[string]any{
				"type": "reaction_added", "user": e.User, "reaction": e.Reaction, "item_user": e.Msg.UserID,
				"item":     map[string]any{"type": "message", "channel": e.Msg.ChannelID, "ts": e.Msg.TS},
				"event_ts": tsOf(now),
			},
		}}
	}
	return nil
}

func postedCandidates(e service.MessagePosted, app service.App, team string) []candidate {
	msg := messageEvent(e.Msg, e.Channel, team)
	out := []candidate{messageCandidate(e.Scope, msg)}
	if e.Msg.UserID != app.BotUserID && mentions(e.Msg.Text, app.BotUserID) {
		mention := map[string]any{
			"type": "app_mention", "channel": e.Msg.ChannelID, "user": e.Msg.UserID, "text": e.Msg.Text,
			"ts": e.Msg.TS, "event_ts": e.Msg.TS, "team": team,
		}
		addOptional(mention, e.Msg)
		restricted := e.Channel.IsPrivate || e.Channel.Kind != service.KindChannel
		out = append(out, candidate{typ: "app_mention", sub: "app_mention", event: mention, needsMember: restricted, targets: e.Targets})
	}
	return out
}

func messageCandidate(sc service.Scope, event map[string]any) candidate {
	return candidate{typ: "message", sub: "message." + subscriptionKind(sc.Channel), event: event, needsMember: true, targets: sc.Targets}
}

// subscriptionKind is the suffix of the message.* bot_events name for a channel.
func subscriptionKind(c service.ChannelInfo) string {
	switch {
	case c.Kind == service.KindIM:
		return "im"
	case c.Kind == service.KindMPIM:
		return "mpim"
	case c.IsPrivate:
		return "groups"
	}
	return "channels"
}

// channelType is the event's channel_type for a channel.
func channelType(c service.ChannelInfo) string {
	switch {
	case c.Kind == service.KindIM:
		return "im"
	case c.Kind == service.KindMPIM:
		return "mpim"
	case c.IsPrivate:
		return "group"
	}
	return "channel"
}

func mentions(text, botUser string) bool {
	return strings.Contains(text, "<@"+botUser+">") || strings.Contains(text, "<@"+botUser+"|")
}

func messageEvent(m service.Message, ch service.ChannelInfo, team string) map[string]any {
	e := map[string]any{
		"type": "message", "channel": m.ChannelID, "user": m.UserID, "text": m.Text, "ts": m.TS,
		"event_ts": m.TS, "channel_type": channelType(ch), "team": team,
	}
	addOptional(e, m)
	if m.Subtype != "" {
		e["subtype"] = m.Subtype
	}
	return e
}

// addOptional sets the fields a message only sometimes has.
func addOptional(e map[string]any, m service.Message) {
	if m.ThreadTS != "" {
		e["thread_ts"] = m.ThreadTS
	}
	if m.BotID != "" {
		e["bot_id"] = m.BotID
	}
	if len(m.Blocks) > 0 {
		e["blocks"] = m.Blocks
	}
}

// messageObject is the nested message of message_changed / message_deleted.
func messageObject(m service.Message, team string) map[string]any {
	o := map[string]any{"type": "message", "user": m.UserID, "text": m.Text, "ts": m.TS, "team": team}
	addOptional(o, m)
	if m.Edited != nil {
		o["edited"] = map[string]any{"user": m.Edited.User, "ts": m.Edited.TS}
	}
	return o
}

func changedEvent(e service.MessageChanged, team string, now time.Time) map[string]any {
	ts := tsOf(now)
	if e.Msg.Edited != nil {
		ts = e.Msg.Edited.TS
	}
	return map[string]any{
		"type": "message", "subtype": "message_changed", "hidden": true, "channel": e.Msg.ChannelID,
		"channel_type": channelType(e.Channel), "ts": ts, "event_ts": ts,
		"message": messageObject(e.Msg, team), "previous_message": messageObject(e.Prev, team),
	}
}

func deletedEvent(e service.MessageDeleted, team string, now time.Time) map[string]any {
	ts := tsOf(now)
	return map[string]any{
		"type": "message", "subtype": "message_deleted", "hidden": true, "channel": e.Msg.ChannelID,
		"channel_type": channelType(e.Channel), "ts": ts, "event_ts": ts, "deleted_ts": e.Msg.TS,
		"previous_message": messageObject(e.Msg, team),
	}
}

// tsOf renders t as a Slack-style "seconds.micros" timestamp.
func tsOf(t time.Time) string { return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000) }

type authorization struct {
	EnterpriseID        *string `json:"enterprise_id"`
	TeamID              string  `json:"team_id"`
	UserID              string  `json:"user_id"`
	IsBot               bool    `json:"is_bot"`
	IsEnterpriseInstall bool    `json:"is_enterprise_install"`
}

type envelope struct {
	Token              string          `json:"token"`
	TeamID             string          `json:"team_id"`
	APIAppID           string          `json:"api_app_id"`
	Event              map[string]any  `json:"event"`
	Type               string          `json:"type"`
	EventID            string          `json:"event_id"`
	EventTime          int64           `json:"event_time"`
	Authorizations     []authorization `json:"authorizations"`
	IsExtSharedChannel bool            `json:"is_ext_shared_channel"`
	EventContext       string          `json:"event_context"`
}

// Envelope wraps an inner event in the event_callback body Slack POSTs to app.
func Envelope(app service.App, team, eventID string, at time.Time, event map[string]any) ([]byte, error) {
	channel, _ := event["channel"].(string)
	etype, _ := event["type"].(string)
	ctx, err := json.Marshal(map[string]string{"et": etype, "tid": team, "aid": app.ID, "cid": channel})
	if err != nil {
		return nil, fmt.Errorf("encode event_context: %w", err)
	}
	body, err := json.Marshal(envelope{
		Token: app.VerificationToken, TeamID: team, APIAppID: app.ID, Event: event, Type: "event_callback",
		EventID: eventID, EventTime: at.Unix(),
		Authorizations: []authorization{{TeamID: team, UserID: app.BotUserID, IsBot: true}},
		EventContext:   "4-" + base64.RawURLEncoding.EncodeToString(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("encode envelope: %w", err)
	}
	return body, nil
}

// MergeEnvelope shallow-merges over onto the JSON object base; a nil value deletes the key.
func MergeEnvelope(base []byte, over map[string]any) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	for k, v := range over {
		if v == nil {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode envelope: %w", err)
	}
	return out, nil
}
