package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"localslack/internal/scenario"
	"localslack/internal/store"
)

// ErrResetIDCollision is returned when the reset seed generates an id that an
// app's bot user already owns. Pick another seed; nothing was changed.
var ErrResetIDCollision = errors.New("service: reset seed collides with an app bot user id")

// ResetOpts selects what a reset keeps and how it repopulates.
type ResetOpts struct {
	// All also removes apps (and their bot users); default keeps them.
	All bool
	// Seed drives the repopulated data and runtime ids; "" picks a random one.
	Seed string
	// Scenario repopulates from this fixture (or its Generate recipe) instead of
	// the demo seed. Problems are reported as *scenario.Error, nothing changes.
	Scenario *scenario.Fixture
}

// ResetCounts is what a reset left behind.
type ResetCounts struct{ Users, Channels, Messages int }

// ResetResult reports a completed reset.
type ResetResult struct {
	Seed   string
	Counts ResetCounts
}

// Reset replaces workspace data (users, channels, messages and anything later
// migrations add) with freshly seeded demo data, or o.Scenario, in one
// transaction. The team row (id) and the ts high-water mark survive, so ids and
// timestamps never run backwards. Without All, apps and their bot users are
// kept (and rejoined to public channels). No events are emitted.
func (s *Service) Reset(ctx context.Context, o ResetOpts) (ResetResult, error) {
	seedStr := o.Seed
	if seedStr == "" {
		var err error
		if seedStr, err = randomSeed(); err != nil {
			return ResetResult{}, err
		}
	}
	var counts store.Counts
	err := s.st.Write(ctx, func(tx *store.Tx) error {
		var err error
		counts, err = s.replaceTx(ctx, tx, o.All, seedStr, o.Scenario)
		return err
	})
	if err != nil {
		return ResetResult{}, err
	}
	if o.All {
		s.tombs.clear() // a blank server forgets revoked tokens
	}
	s.ids.Reseed(seedStr + "/runtime") // distinct from the populate stream, so new ids never replay seeded ones
	if err := s.RefreshApps(ctx); err != nil {
		return ResetResult{}, err
	}
	return ResetResult{Seed: seedStr, Counts: ResetCounts{Users: counts.Users, Channels: counts.Conversations, Messages: counts.Messages}}, nil
}

// Restore replaces the workspace with fx (a snapshot or scenario) the way a
// partial reset does: apps and their bot users are kept and rejoined to public
// channels, the team id and ts high-water mark survive, and no events are
// emitted. Runtime id generation is left alone. Problems are *scenario.Error.
func (s *Service) Restore(ctx context.Context, fx scenario.Fixture) (ResetCounts, error) {
	seedStr, err := randomSeed() // only for ids a hand-written fixture leaves out
	if err != nil {
		return ResetCounts{}, err
	}
	var counts store.Counts
	err = s.st.Write(ctx, func(tx *store.Tx) error {
		var err error
		counts, err = s.replaceTx(ctx, tx, false, seedStr, &fx)
		return err
	})
	if err != nil {
		return ResetCounts{}, err
	}
	return ResetCounts{Users: counts.Users, Channels: counts.Conversations, Messages: counts.Messages}, nil
}

func (s *Service) replaceTx(ctx context.Context, tx *store.Tx, all bool, seedStr string, sc *scenario.Fixture) (store.Counts, error) {
	keep := []string{"teams"}
	var botUsers []store.User
	if !all {
		keep = append(keep, "apps")
		var err error
		if botUsers, err = appBotUsers(ctx, tx); err != nil {
			return store.Counts{}, err
		}
	}
	fx, err := prepareFixture(sc, seedStr, s.now(), reservedOf(botUsers))
	if err != nil {
		return store.Counts{}, err
	}
	if err := tx.WipeWorkspace(ctx, keep...); err != nil {
		return store.Counts{}, err
	}
	if err := writeFixture(ctx, tx, fx); err != nil {
		return store.Counts{}, err
	}
	for _, u := range botUsers {
		if err := tx.InsertUser(ctx, u); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return store.Counts{}, fmt.Errorf("bot user %s: %w", u.ID, ErrResetIDCollision)
			}
			return store.Counts{}, err
		}
		if slices.Contains(fx.AppBots, u.ID) { // the fixture states its membership exactly
			continue
		}
		if err := JoinBotToPublicChannels(ctx, tx, u.ID); err != nil {
			return store.Counts{}, err
		}
	}
	maxTS, err := tx.MaxTS(ctx)
	if err != nil {
		return store.Counts{}, err
	}
	if err := s.ts.advanceTo(maxTS); err != nil {
		return store.Counts{}, err
	}
	return tx.Counts(ctx)
}

// appBotUsers returns the stored bot user row of every app (missing rows are skipped).
func appBotUsers(ctx context.Context, tx *store.Tx) ([]store.User, error) {
	apps, err := tx.Apps(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.User
	for _, a := range apps {
		u, err := tx.User(ctx, a.BotUserID)
		switch {
		case errors.Is(err, store.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			out = append(out, u)
		}
	}
	return out, nil
}
