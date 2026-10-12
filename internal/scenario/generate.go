package scenario

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"localslack/internal/ids"
)

// Generate limits and defaults (zero fields take the default).
const (
	defaultUsers    = 10
	defaultChannels = 5
	defaultMessages = 20
	maxUsers        = 2000
	maxChannels     = 1000
	maxMessages     = 5000
	maxTotal        = 200000
	defaultSeedName = "localslack"
)

func (g Generate) withDefaults() Generate {
	g.Users = cmpOr(g.Users, defaultUsers)
	g.Channels = cmpOr(g.Channels, defaultChannels)
	g.MessagesPerChannel = cmpOr(g.MessagesPerChannel, defaultMessages)
	if g.Seed == "" {
		g.Seed = defaultSeedName
	}
	return g
}

func cmpOr(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// Issues lists what is wrong with g's parameters, at pointers under at.
func (g Generate) Issues(at string) []Issue {
	var out []Issue
	add := func(field, msg string) { out = append(out, Issue{Pointer: at + "/" + field, Message: msg}) }
	if g.Users < 0 || g.Users > maxUsers {
		add("users", fmt.Sprintf("users must be 1..%d", maxUsers))
	}
	if g.Channels < 0 || g.Channels > maxChannels {
		add("channels", fmt.Sprintf("channels must be 1..%d", maxChannels))
	}
	if g.MessagesPerChannel < 0 || g.MessagesPerChannel > maxMessages {
		add("messages_per_channel", fmt.Sprintf("messages_per_channel must be 1..%d", maxMessages))
	}
	if math.IsNaN(g.ThreadRatio) || g.ThreadRatio < 0 || g.ThreadRatio > 1 {
		add("thread_ratio", "thread_ratio must be between 0 and 1")
	}
	d := g.withDefaults()
	if g.PrivateChannels < 0 || (len(out) == 0 && g.PrivateChannels >= d.Channels) {
		add("private_channels", "private_channels must be less than channels (general is public)")
	}
	if len(out) == 0 && d.Channels*d.MessagesPerChannel > maxTotal {
		out = append(out, Issue{Pointer: at, Message: fmt.Sprintf("channels x messages_per_channel must be at most %d", maxTotal)})
	}
	return out
}

// Expand expands g into a complete fixture: g.Users humans plus one bot,
// "general" first, the last g.PrivateChannels channels private, threads and
// reactions sprinkled in. Same g and now give the same fixture; ts values end
// just before now. Public channels hold everyone.
func Expand(g Generate, now time.Time) (Fixture, error) {
	if issues := g.Issues("/generate"); len(issues) > 0 {
		return Fixture{}, &Error{Issues: issues}
	}
	g = g.withDefaults()
	b := newBuilder(g, now)
	b.users()
	b.channels()
	b.messages()
	return b.fx, nil
}

type builder struct {
	g   Generate
	now time.Time
	ids *ids.Generator
	rng *rand.Rand
	fx  Fixture
}

func newBuilder(g Generate, now time.Time) *builder {
	h := fnv.New64a()
	_, _ = h.Write([]byte(g.Seed))
	b := &builder{g: g, now: now, ids: ids.NewGenerator(g.Seed), rng: rand.New(rand.NewPCG(h.Sum64(), 2))}
	b.fx = Fixture{Version: Version, Team: &Team{ID: b.ids.Team(), Name: "LocalSlack", Domain: "localslack"}}
	return b
}

func (b *builder) users() {
	pool := People()
	for i := range b.g.Users {
		name := pool[i%len(pool)]
		if round := i / len(pool); round > 0 {
			name += strconv.Itoa(round + 1)
		}
		b.fx.Users = append(b.fx.Users, User{ID: b.ids.User(), Name: name, RealName: name})
	}
	bots := ids.NewGenerator(b.g.Seed + "/bot")
	b.fx.Users = append(b.fx.Users, User{ID: b.ids.User(), Name: BotName, RealName: BotName, IsBot: true, BotID: bots.Bot()})
}

func (b *builder) channels() {
	topics := Topics()
	firstPrivate := b.g.Channels - b.g.PrivateChannels
	for i := range b.g.Channels {
		name := "general"
		if i > 0 {
			name = "channel-" + strconv.Itoa(i)
			if i-1 < len(topics) {
				name = topics[i-1]
			}
		}
		c := Channel{Name: name, Creator: b.fx.Users[0].ID, Created: b.now.Unix(), IsPrivate: i >= firstPrivate}
		c.ID = b.ids.Channel()
		if c.IsPrivate {
			c.ID = b.ids.Group()
			c.Members = b.subset()
		} else {
			c.Members = b.everyone()
		}
		b.fx.Channels = append(b.fx.Channels, c)
	}
}

func (b *builder) everyone() []string {
	out := make([]string, len(b.fx.Users))
	for i, u := range b.fx.Users {
		out[i] = u.ID
	}
	return out
}

// subset picks 2.. a proper subset of users, in user order.
func (b *builder) subset() []string {
	n := len(b.fx.Users)
	size := min(2+b.rng.IntN(max(1, n/2)), n-1)
	picked := b.rng.Perm(n)[:size]
	slices.Sort(picked)
	out := make([]string, size)
	for i, p := range picked {
		out[i] = b.fx.Users[p].ID
	}
	return out
}

// messages interleaves channels in time (slot-major) but emits channel-major.
func (b *builder) messages() {
	per, chans := b.g.MessagesPerChannel, len(b.fx.Channels)
	total := per * chans
	base := b.now.Unix() - int64(total)
	bots := map[string]string{}
	for _, u := range b.fx.Users {
		if u.IsBot {
			bots[u.ID] = u.BotID
		}
	}
	texts := Lines()
	for c, ch := range b.fx.Channels {
		var roots []string
		for s := range per {
			k := s*chans + c
			m := Message{
				Channel: ch.ID, TS: fmt.Sprintf("%d.%06d", base+int64(k), k%999999+1),
				User: ch.Members[b.rng.IntN(len(ch.Members))], Text: texts[b.rng.IntN(len(texts))],
				BotID: "",
			}
			m.BotID = bots[m.User]
			if len(roots) > 0 && b.rng.Float64() < b.g.ThreadRatio {
				m.ThreadTS = roots[b.rng.IntN(len(roots))]
			} else {
				roots = append(roots, m.TS)
			}
			if b.rng.IntN(10) == 0 {
				m.Reactions = b.reactions(ch.Members)
			}
			b.fx.Messages = append(b.fx.Messages, m)
		}
	}
}

func (b *builder) reactions(members []string) []Reaction {
	pool := Emoji()
	n := 1 + b.rng.IntN(2)
	var out []Reaction
	for _, e := range b.rng.Perm(len(pool))[:n] {
		k := 1 + b.rng.IntN(min(3, len(members)))
		var users []string
		for _, u := range b.rng.Perm(len(members))[:k] {
			users = append(users, members[u])
		}
		out = append(out, Reaction{Name: pool[e], Users: users})
	}
	return out
}
