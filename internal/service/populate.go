package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"localslack/internal/ids"
	"localslack/internal/scenario"
	"localslack/internal/seed"
	"localslack/internal/store"
)

// SeedInput describes first-run population of an empty database.
type SeedInput struct {
	// Fixture is the workspace to create; nil seeds the deterministic demo data.
	Fixture *scenario.Fixture
	// Seed drives the demo data and any ids a Fixture leaves out.
	Seed string
	Now  time.Time
}

// SeedEmpty populates st from in and reports whether it did; a database that
// already has a team is left alone. Validation failures are *scenario.Error.
func SeedEmpty(ctx context.Context, st *store.Store, in SeedInput) (bool, error) {
	did := false
	err := st.Write(ctx, func(tx *store.Tx) error {
		empty, err := tx.IsEmpty(ctx)
		if err != nil || !empty {
			return err
		}
		fx, err := prepareFixture(in.Fixture, in.Seed, in.Now, scenario.Reserved{})
		if err != nil {
			return err
		}
		did = true
		return writeFixture(ctx, tx, fx)
	})
	if err != nil {
		return false, fmt.Errorf("seed: %w", err)
	}
	return did, nil
}

// prepareFixture turns sc into a complete, valid fixture: nil is the demo seed
// (trusted), a Generate recipe is expanded, hand-written conveniences are
// resolved, then everything is validated against the reserved identities.
func prepareFixture(sc *scenario.Fixture, seedStr string, now time.Time, res scenario.Reserved) (scenario.Fixture, error) {
	if sc == nil {
		return seed.Fixture(seed.Options{Seed: seedStr, Now: now}), nil
	}
	fx := *sc
	if fx.Version == 0 {
		fx.Version = scenario.Version
	}
	if fx.Generate != nil {
		if issues := scenario.Validate(fx, res); len(issues) > 0 {
			return scenario.Fixture{}, &scenario.Error{Issues: issues}
		}
		g := *fx.Generate
		if g.Seed == "" {
			g.Seed = seedStr
		}
		var err error
		if fx, err = scenario.Expand(g, now); err != nil {
			return scenario.Fixture{}, err
		}
	}
	fx = scenario.Resolve(fx, ids.NewGenerator(seedStr+"/scenario"), now)
	if issues := scenario.Validate(fx, res); len(issues) > 0 {
		return scenario.Fixture{}, &scenario.Error{Issues: issues}
	}
	return fx, nil
}

func reservedOf(botUsers []store.User) scenario.Reserved {
	var r scenario.Reserved
	for _, u := range botUsers {
		r.UserIDs = append(r.UserIDs, u.ID)
		r.UserNames = append(r.UserNames, u.Name)
		if u.BotID != "" {
			r.BotIDs = append(r.BotIDs, u.BotID)
		}
	}
	return r
}

// writeFixture inserts a prepared fixture into tx with batched prepared
// statements. The team row is created if absent; an existing one keeps its id
// (apps and sessions reference it) and takes the fixture's name and domain.
// Thread reply counts are derived; reactions are stamped with their message's
// time. No events are emitted.
func writeFixture(ctx context.Context, tx *store.Tx, fx scenario.Fixture) error {
	team, err := ensureTeam(ctx, tx, *fx.Team)
	if err != nil {
		return err
	}
	b, err := tx.Bulk(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	for _, u := range fx.Users {
		if err := b.User(ctx, store.User{ID: u.ID, TeamID: team.ID, Name: u.Name, RealName: u.RealName, IsBot: u.IsBot, BotID: u.BotID}); err != nil {
			return err
		}
	}
	for _, c := range fx.Channels {
		row := store.Conversation{
			ID: c.ID, TeamID: team.ID, Name: c.Name, Kind: KindChannel, Creator: c.Creator, Created: c.Created,
			IsArchived: c.IsArchived, IsPrivate: c.IsPrivate,
		}
		if err := writeConversation(ctx, b, row, c.Members); err != nil {
			return err
		}
	}
	for _, d := range fx.DMs {
		kind := KindIM
		if len(d.Members) > 2 {
			kind = KindMPIM
		}
		row := store.Conversation{ID: d.ID, TeamID: team.ID, Kind: kind, Creator: d.Creator, Created: d.Created}
		if err := writeConversation(ctx, b, row, d.Members); err != nil {
			return err
		}
	}
	return writeMessages(ctx, b, fx.Messages)
}

func ensureTeam(ctx context.Context, tx *store.Tx, want scenario.Team) (store.Team, error) {
	have, err := tx.Team(ctx)
	switch {
	case errors.Is(err, store.ErrNotFound):
		t := store.Team{ID: want.ID, Name: want.Name, Domain: want.Domain}
		return t, tx.InsertTeam(ctx, t)
	case err != nil:
		return store.Team{}, err
	case have.Name != want.Name || have.Domain != want.Domain:
		have.Name, have.Domain = want.Name, want.Domain
		return have, tx.RenameTeam(ctx, want.Name, want.Domain)
	}
	return have, nil
}

func writeConversation(ctx context.Context, b *store.Bulk, c store.Conversation, members []string) error {
	if err := b.Conversation(ctx, c); err != nil {
		return err
	}
	for _, m := range members {
		if err := b.Member(ctx, c.ID, m); err != nil {
			return err
		}
	}
	return nil
}

type threadKey struct{ channel, root string }

type threadStat struct {
	count  int
	latest string
}

func threadStats(msgs []scenario.Message) map[threadKey]threadStat {
	stats := map[threadKey]threadStat{}
	for _, m := range msgs {
		if m.ThreadTS == "" || m.ThreadTS == m.TS {
			continue
		}
		k := threadKey{m.Channel, m.ThreadTS}
		st := stats[k]
		st.count++
		if st.latest == "" || tsLess(st.latest, m.TS) {
			st.latest = m.TS
		}
		stats[k] = st
	}
	return stats
}

// tsLess compares validated Slack timestamps numerically.
func tsLess(a, b string) bool {
	ua, _ := parseTS(a)
	ub, _ := parseTS(b)
	return ua < ub
}

func writeMessages(ctx context.Context, b *store.Bulk, msgs []scenario.Message) error {
	stats := threadStats(msgs)
	for _, m := range msgs {
		st := stats[threadKey{m.Channel, m.TS}]
		row := store.Message{
			ChannelID: m.Channel, TS: m.TS, UserID: m.User, Text: m.Text, ThreadTS: m.ThreadTS,
			ReplyCount: st.count, LatestReply: st.latest,
			BotID: m.BotID, Subtype: m.Subtype, Blocks: m.Blocks, Attachments: m.Attachments, Metadata: m.Metadata,
		}
		if m.Edited != nil {
			row.Edited = &store.Edit{User: m.Edited.User, TS: m.Edited.TS}
		}
		if err := b.Message(ctx, row); err != nil {
			return err
		}
		created := tsSeconds(m.TS)
		for _, r := range m.Reactions {
			for _, u := range r.Users {
				if err := b.Reaction(ctx, store.Reaction{ChannelID: m.Channel, TS: m.TS, Name: r.Name, UserID: u, Created: created}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func tsSeconds(ts string) int64 {
	sec, _, _ := strings.Cut(ts, ".")
	n, _ := strconv.ParseInt(sec, 10, 64)
	return n
}
