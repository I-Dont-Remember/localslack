package scenario

import (
	"slices"
	"strings"
	"time"
)

// IDSource hands out fresh ids (ids.Generator satisfies it).
type IDSource interface {
	Team() string
	User() string
	Bot() string
	Channel() string
	DM() string
	Group() string
}

// Resolve returns fx with the conveniences of hand-written files made explicit:
// a missing team becomes LocalSlack, missing team/user/bot/channel/DM ids are drawn from src (in that order, so the
// result is deterministic per src), channel and user references given by name
// ("general", "#general", "ann", "@ann") become ids, a zero channel created
// becomes now, and a root message's thread_ts equal to its own ts is dropped.
// References that match nothing are left for Validate to report. fx is not modified.
func Resolve(fx Fixture, src IDSource, now time.Time) Fixture {
	out := fx
	t := Team{Name: "LocalSlack", Domain: "localslack"}
	if fx.Team != nil {
		t = *fx.Team
	}
	if t.ID == "" {
		t.ID = src.Team()
	}
	out.Team = &t
	out.Users = slices.Clone(fx.Users)
	for i := range out.Users {
		u := &out.Users[i]
		if u.ID == "" {
			u.ID = src.User()
		}
		if u.IsBot && u.BotID == "" {
			u.BotID = src.Bot()
		}
	}
	r := newRefs(out.Users)
	out.Channels = resolveChannels(fx.Channels, r, src, now)
	out.DMs = resolveDMs(fx.DMs, r, src, now)
	r.addConversations(out.Channels, out.DMs)
	out.Messages = resolveMessages(fx.Messages, r)
	return out
}

// refs maps names to ids; ids map to themselves.
type refs struct{ users, convs map[string]string }

func newRefs(users []User) *refs {
	r := &refs{users: map[string]string{}, convs: map[string]string{}}
	for _, u := range users {
		r.users[u.ID] = u.ID
	}
	for _, u := range users {
		if _, taken := r.users[u.Name]; !taken {
			r.users[u.Name] = u.ID
		}
	}
	return r
}

func (r *refs) addConversations(chs []Channel, dms []DM) {
	for _, c := range chs {
		r.convs[c.ID] = c.ID
	}
	for _, d := range dms {
		r.convs[d.ID] = d.ID
	}
	for _, c := range chs {
		if _, taken := r.convs[c.Name]; !taken {
			r.convs[c.Name] = c.ID
		}
	}
}

func (r *refs) user(s string) string {
	if id, ok := r.users[strings.TrimPrefix(s, "@")]; ok {
		return id
	}
	return s
}

func (r *refs) conv(s string) string {
	if id, ok := r.convs[strings.TrimPrefix(s, "#")]; ok {
		return id
	}
	return s
}

func (r *refs) userList(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = r.user(s)
	}
	return out
}

func resolveChannels(in []Channel, r *refs, src IDSource, now time.Time) []Channel {
	out := slices.Clone(in)
	for i := range out {
		c := &out[i]
		switch {
		case c.ID != "":
		case c.IsPrivate:
			c.ID = src.Group()
		default:
			c.ID = src.Channel()
		}
		if c.Created == 0 {
			c.Created = now.Unix()
		}
		c.Creator = r.user(c.Creator)
		c.Members = r.userList(c.Members)
	}
	return out
}

func resolveDMs(in []DM, r *refs, src IDSource, now time.Time) []DM {
	out := slices.Clone(in)
	for i := range out {
		d := &out[i]
		d.Members = r.userList(d.Members)
		d.Creator = r.user(d.Creator)
		if d.Created == 0 {
			d.Created = now.Unix()
		}
		switch {
		case d.ID != "":
		case len(d.Members) > 2:
			d.ID = src.Group()
		default:
			d.ID = src.DM()
		}
	}
	return out
}

func resolveMessages(in []Message, r *refs) []Message {
	out := slices.Clone(in)
	for i := range out {
		m := &out[i]
		m.Channel = r.conv(m.Channel)
		m.User = r.user(m.User)
		if m.ThreadTS == m.TS {
			m.ThreadTS = ""
		}
		if m.Edited != nil {
			e := *m.Edited
			e.User = r.user(e.User)
			m.Edited = &e
		}
		if m.Reactions != nil {
			m.Reactions = slices.Clone(m.Reactions)
			for j := range m.Reactions {
				m.Reactions[j].Users = r.userList(m.Reactions[j].Users)
			}
		}
	}
	return out
}
