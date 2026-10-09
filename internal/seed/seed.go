// Package seed populates an empty database with deterministic demo data.
package seed

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"time"

	"localslack/internal/ids"
	"localslack/internal/store"
)

// Options controls seeding. The same Seed and Now yield identical data.
type Options struct {
	Seed string
	Now  time.Time
}

const botName = "localslack-bot"

func people() []string {
	return []string{"ada", "grace", "linus", "margaret", "alan", "barbara", "dennis", "ken", "radia", "tim"}
}

func topics() []string {
	return []string{
		"random", "engineering", "design", "product", "support", "sales", "marketing", "ops", "security", "data",
		"frontend", "backend", "mobile", "infra", "qa", "docs", "hiring", "finance", "legal", "people",
		"announcements", "standup", "retro", "incidents", "oncall", "releases", "roadmap", "research", "ideas", "water-cooler",
		"music", "books", "games", "food", "travel", "fitness", "pets", "memes", "help", "feedback",
		"ci", "deploys", "alerts", "metrics", "analytics", "partners", "customers", "onboarding", "training",
	}
}

func lines() []string {
	return []string{
		"morning all", "anyone seen the build?", "shipping this today", "can someone review my PR?",
		"lunch at noon?", "looks good to me", "nice work!", "I'll take a look", "blocked on the API change",
		"standup in 5", "pushed a fix", "thanks!", "ok let's sync later",
	}
}

// Apply seeds an empty store and reports whether it did; a populated store is left alone.
func Apply(ctx context.Context, s *store.Store, o Options) (bool, error) {
	empty, err := s.IsEmpty(ctx)
	if err != nil || !empty {
		return false, err
	}
	g := newGen(o)
	if err := s.Write(ctx, g.populate(ctx)); err != nil {
		return false, fmt.Errorf("seed: %w", err)
	}
	return true, nil
}

type gen struct {
	ids *ids.Generator
	rng *rand.Rand
	now time.Time
}

func newGen(o Options) *gen {
	h := fnv.New64a()
	_, _ = h.Write([]byte(o.Seed))
	return &gen{
		ids: ids.NewGenerator(o.Seed),
		rng: rand.New(rand.NewPCG(h.Sum64(), 1)),
		now: o.Now,
	}
}

func (g *gen) populate(ctx context.Context) func(*store.Tx) error {
	return func(tx *store.Tx) error {
		team := store.Team{ID: g.ids.Team(), Name: "LocalSlack", Domain: "localslack"}
		if err := tx.InsertTeam(ctx, team); err != nil {
			return err
		}
		users, err := g.users(ctx, tx, team.ID)
		if err != nil {
			return err
		}
		convs, err := g.channels(ctx, tx, team.ID, users)
		if err != nil {
			return err
		}
		return g.thread(ctx, tx, convs[0].ID, users)
	}
}

func (g *gen) users(ctx context.Context, tx *store.Tx, teamID string) ([]store.User, error) {
	pool := people()
	n := 1 + g.rng.IntN(len(pool))
	var users []store.User
	for _, i := range g.rng.Perm(len(pool))[:n] {
		users = append(users, store.User{ID: g.ids.User(), TeamID: teamID, Name: pool[i], RealName: pool[i]})
	}
	users = append(users, store.User{ID: g.ids.User(), TeamID: teamID, Name: botName, RealName: botName, IsBot: true})
	for _, u := range users {
		if err := tx.InsertUser(ctx, u); err != nil {
			return nil, err
		}
	}
	return users, nil
}

// channels creates #general (always first) plus 1..49 others, everyone a member.
func (g *gen) channels(ctx context.Context, tx *store.Tx, teamID string, users []store.User) ([]store.Conversation, error) {
	pool := topics()
	extra := 1 + g.rng.IntN(len(pool))
	names := []string{"general"}
	for _, i := range g.rng.Perm(len(pool))[:extra] {
		names = append(names, pool[i])
	}
	var convs []store.Conversation
	for _, name := range names {
		c := store.Conversation{
			ID: g.ids.Channel(), TeamID: teamID, Name: name, Kind: store.KindChannel,
			Creator: users[0].ID, Created: g.now.Unix(),
		}
		if err := tx.InsertConversation(ctx, c); err != nil {
			return nil, err
		}
		for _, u := range users {
			if err := tx.AddMember(ctx, c.ID, u.ID); err != nil {
				return nil, err
			}
		}
		convs = append(convs, c)
	}
	return convs, nil
}

// thread writes 3..25 messages to a channel; k consecutive ones reply to a parent.
func (g *gen) thread(ctx context.Context, tx *store.Tx, channelID string, users []store.User) error {
	n := 3 + g.rng.IntN(23)
	k := 1 + g.rng.IntN(min(3, n-2))
	parent := g.rng.IntN(n - k)
	texts := lines()
	base := g.now.Add(-time.Hour).Unix()
	tsAt := func(i int) string { return fmt.Sprintf("%d.%06d", base+int64(i)*60, i+1) }

	for i := range n {
		m := store.Message{
			ChannelID: channelID, TS: tsAt(i), UserID: users[g.rng.IntN(len(users))].ID,
			Text: texts[g.rng.IntN(len(texts))],
		}
		switch {
		case i == parent:
			m.ReplyCount, m.LatestReply = k, tsAt(parent+k)
		case i > parent && i <= parent+k:
			m.ThreadTS = tsAt(parent)
		}
		if err := tx.InsertMessage(ctx, m); err != nil {
			return err
		}
	}
	return nil
}
