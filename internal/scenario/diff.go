package scenario

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Delta is what changed between two fixtures (GET /_localslack/diff). Every
// list is non-nil and sorted by key, so output is deterministic.
type Delta struct {
	Users         Section[User]         `json:"users"`
	Conversations Section[Conversation] `json:"conversations"`
	Members       Section[Member]       `json:"members"`
	Messages      Section[Message]      `json:"messages"`
	Reactions     Section[ReactionRef]  `json:"reactions"`
}

// Section is one entity kind's added, removed and changed items.
type Section[T any] struct {
	Added   []T      `json:"added"`
	Removed []T      `json:"removed"`
	Changed []Change `json:"changed"`
}

// Change is one entity present on both sides with differing fields. Users and
// conversations are keyed by ID, messages by Channel+TS.
type Change struct {
	ID      string                 `json:"id,omitempty"`
	Channel string                 `json:"channel,omitempty"`
	TS      string                 `json:"ts,omitempty"`
	Fields  map[string]FieldChange `json:"fields"`
}

// FieldChange is a field's value before and after.
type FieldChange struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// Conversation is a channel or DM as the diff sees it.
type Conversation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Creator    string `json:"creator"`
	Created    int64  `json:"created"`
	IsArchived bool   `json:"is_archived"`
	IsPrivate  bool   `json:"is_private"`
}

// Member is one user's membership of one conversation.
type Member struct {
	Channel string `json:"channel"`
	User    string `json:"user"`
}

// ReactionRef is one user's reaction on one message.
type ReactionRef struct {
	Channel string `json:"channel"`
	TS      string `json:"ts"`
	Name    string `json:"name"`
	User    string `json:"user"`
}

// Diff compares two fixtures. Members and reactions are sets (added/removed
// only, Changed stays empty); messages in added or removed conversations are
// listed too. Messages in added/removed lists carry no reactions: those are in
// the reactions section.
func Diff(from, to Fixture) Delta {
	a, b := indexOf(from), indexOf(to)
	return Delta{
		Users:         keyed(a.users, b.users, userFields, func(k string, f map[string]FieldChange) Change { return Change{ID: k, Fields: f} }),
		Conversations: keyed(a.convs, b.convs, convFields, func(k string, f map[string]FieldChange) Change { return Change{ID: k, Fields: f} }),
		Members:       sets(a.members, b.members),
		Messages:      keyed(a.msgs, b.msgs, msgFields, msgChange),
		Reactions:     sets(a.reacts, b.reacts),
	}
}

const keySep = "\x00"

type index struct {
	users   map[string]User
	convs   map[string]Conversation
	members map[string]Member
	msgs    map[string]Message
	reacts  map[string]ReactionRef
}

func indexOf(fx Fixture) index {
	ix := index{
		users: map[string]User{}, convs: map[string]Conversation{}, members: map[string]Member{},
		msgs: map[string]Message{}, reacts: map[string]ReactionRef{},
	}
	for _, u := range fx.Users {
		ix.users[u.ID] = u
	}
	for _, c := range fx.Channels {
		ix.convs[c.ID] = Conversation{ID: c.ID, Name: c.Name, Kind: "channel", Creator: c.Creator, Created: c.Created, IsArchived: c.IsArchived, IsPrivate: c.IsPrivate}
		ix.addMembers(c.ID, c.Members)
	}
	for _, d := range fx.DMs {
		kind := "im"
		if len(d.Members) > 2 {
			kind = "mpim"
		}
		ix.convs[d.ID] = Conversation{ID: d.ID, Kind: kind, Creator: d.Creator, Created: d.Created}
		ix.addMembers(d.ID, d.Members)
	}
	for _, m := range fx.Messages {
		for _, r := range m.Reactions {
			for _, u := range r.Users {
				ix.reacts[strings.Join([]string{m.Channel, m.TS, r.Name, u}, keySep)] = ReactionRef{Channel: m.Channel, TS: m.TS, Name: r.Name, User: u}
			}
		}
		m.Reactions = nil
		ix.msgs[m.Channel+keySep+m.TS] = m
	}
	return ix
}

func (ix index) addMembers(conv string, users []string) {
	for _, u := range users {
		ix.members[conv+keySep+u] = Member{Channel: conv, User: u}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

func sets[V any](a, b map[string]V) Section[V] {
	s := Section[V]{Added: []V{}, Removed: []V{}, Changed: []Change{}}
	for _, k := range sortedKeys(b) {
		if _, ok := a[k]; !ok {
			s.Added = append(s.Added, b[k])
		}
	}
	for _, k := range sortedKeys(a) {
		if _, ok := b[k]; !ok {
			s.Removed = append(s.Removed, a[k])
		}
	}
	return s
}

func keyed[V any](a, b map[string]V, fields func(V) map[string]any, change func(string, map[string]FieldChange) Change) Section[V] {
	s := sets(a, b)
	for _, k := range sortedKeys(b) {
		old, ok := a[k]
		if !ok {
			continue
		}
		if f := fieldDiff(fields(old), fields(b[k])); len(f) > 0 {
			s.Changed = append(s.Changed, change(k, f))
		}
	}
	return s
}

func fieldDiff(a, b map[string]any) map[string]FieldChange {
	out := map[string]FieldChange{}
	for k, av := range a {
		if bv := b[k]; !reflect.DeepEqual(av, bv) {
			out[k] = FieldChange{From: av, To: bv}
		}
	}
	return out
}

func userFields(u User) map[string]any {
	return map[string]any{"name": u.Name, "real_name": u.RealName, "is_bot": u.IsBot, "bot_id": u.BotID}
}

func convFields(c Conversation) map[string]any {
	return map[string]any{
		"name": c.Name, "kind": c.Kind, "creator": c.Creator, "created": c.Created,
		"is_archived": c.IsArchived, "is_private": c.IsPrivate,
	}
}

func msgFields(m Message) map[string]any {
	var edited any
	if m.Edited != nil {
		edited = map[string]any{"user": m.Edited.User, "ts": m.Edited.TS}
	}
	return map[string]any{
		"user": m.User, "text": m.Text, "thread_ts": m.ThreadTS, "bot_id": m.BotID, "subtype": m.Subtype,
		"blocks": canonical(m.Blocks), "attachments": canonical(m.Attachments), "metadata": canonical(m.Metadata),
		"edited": edited,
	}
}

func msgChange(k string, f map[string]FieldChange) Change {
	channel, ts, _ := strings.Cut(k, keySep)
	return Change{Channel: channel, TS: ts, Fields: f}
}

// canonical decodes raw JSON so whitespace and key order do not count as changes.
func canonical(raw json.RawMessage) any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
