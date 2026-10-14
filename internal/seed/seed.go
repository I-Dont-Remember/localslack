// Package seed builds the deterministic demo workspace as a scenario Fixture.
// It does not touch the database: the service populates the fixture, so the
// default seed, --scenario files, resets and snapshots share one code path.
package seed

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"time"

	"localslack/internal/ids"
	"localslack/internal/scenario"
)

// Options controls seeding. The same Seed and Now yield an identical fixture.
type Options struct {
	Seed string
	Now  time.Time
}

// Fixture returns the demo workspace for o: 1..10 people plus the workspace
// bot, #general plus 1..49 other channels (everyone a member), and one thread
// of 3..25 messages in #general. The RNG draw order is frozen so ids and
// content for a given seed never change.
func Fixture(o Options) scenario.Fixture {
	g := newGen(o)
	fx := scenario.Fixture{
		Version: scenario.Version,
		Team:    &scenario.Team{ID: g.ids.Team(), Name: "LocalSlack", Domain: "localslack"},
	}
	fx.Users = g.users()
	fx.Channels = g.channels(fx.Users)
	fx.Messages = g.thread(fx.Channels[0].ID, fx.Users)
	return fx
}

type gen struct {
	seed string
	ids  *ids.Generator
	rng  *rand.Rand
	now  time.Time
}

func newGen(o Options) *gen {
	h := fnv.New64a()
	_, _ = h.Write([]byte(o.Seed))
	return &gen{
		ids: ids.NewGenerator(o.Seed),
		rng: rand.New(rand.NewPCG(h.Sum64(), 1)),
		now: o.Now, seed: o.Seed,
	}
}

func (g *gen) users() []scenario.User {
	pool := scenario.People()
	n := 1 + g.rng.IntN(len(pool))
	var users []scenario.User
	for _, i := range g.rng.Perm(len(pool))[:n] {
		users = append(users, scenario.User{ID: g.ids.User(), Name: pool[i], RealName: pool[i]})
	}
	// bot ids come from their own stream so adding them leaves every other seeded id unchanged.
	bots := ids.NewGenerator(g.seed + "/bot")
	return append(users, scenario.User{ID: g.ids.User(), Name: scenario.BotName, RealName: scenario.BotName, IsBot: true, BotID: bots.Bot()})
}

// channels creates #general (always first) plus 1..49 others, everyone a member.
func (g *gen) channels(users []scenario.User) []scenario.Channel {
	pool := scenario.Topics()
	extra := 1 + g.rng.IntN(len(pool))
	names := []string{"general"}
	for _, i := range g.rng.Perm(len(pool))[:extra] {
		names = append(names, pool[i])
	}
	members := make([]string, len(users))
	for i, u := range users {
		members[i] = u.ID
	}
	var chans []scenario.Channel
	for _, name := range names {
		chans = append(chans, scenario.Channel{
			ID: g.ids.Channel(), Name: name, Creator: users[0].ID, Created: g.now.Unix(), Members: members,
		})
	}
	return chans
}

// thread writes 3..25 messages to a channel; k consecutive ones reply to a parent.
func (g *gen) thread(channelID string, users []scenario.User) []scenario.Message {
	n := 3 + g.rng.IntN(23)
	k := 1 + g.rng.IntN(min(3, n-2))
	parent := g.rng.IntN(n - k)
	texts := scenario.Lines()
	base := g.now.Add(-time.Hour).Unix()
	tsAt := func(i int) string { return fmt.Sprintf("%d.%06d", base+int64(i)*60, i+1) }

	msgs := make([]scenario.Message, 0, n)
	for i := range n {
		m := scenario.Message{
			Channel: channelID, TS: tsAt(i), User: users[g.rng.IntN(len(users))].ID,
			Text: texts[g.rng.IntN(len(texts))],
		}
		if i > parent && i <= parent+k {
			m.ThreadTS = tsAt(parent)
		}
		msgs = append(msgs, m)
	}
	return msgs
}
