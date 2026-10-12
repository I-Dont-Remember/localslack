package service

import (
	"context"
	"errors"
	"fmt"

	"localslack/internal/seed"
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
}

// ResetCounts is what a reset left behind.
type ResetCounts struct{ Users, Channels, Messages int }

// ResetResult reports a completed reset.
type ResetResult struct {
	Seed   string
	Counts ResetCounts
}

// Reset replaces workspace data (users, channels, messages and anything later
// migrations add) with freshly seeded demo data in one transaction. The team row
// and the ts high-water mark survive, so ids and timestamps never run backwards.
// Without All, apps and their bot users are kept. No events are emitted.
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
		counts, err = s.resetTx(ctx, tx, o.All, seedStr)
		return err
	})
	if err != nil {
		return ResetResult{}, err
	}
	s.ids.Reseed(seedStr + "/runtime") // distinct from the populate stream, so new ids never replay seeded ones
	if err := s.RefreshApps(ctx); err != nil {
		return ResetResult{}, err
	}
	return ResetResult{Seed: seedStr, Counts: ResetCounts{Users: counts.Users, Channels: counts.Conversations, Messages: counts.Messages}}, nil
}

func (s *Service) resetTx(ctx context.Context, tx *store.Tx, all bool, seedStr string) (store.Counts, error) {
	var team *store.Team
	if t, err := tx.Team(ctx); err == nil {
		team = &t
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Counts{}, err
	}
	keep := []string{"teams"}
	var botUsers []store.User
	if !all {
		keep = append(keep, "apps")
		var err error
		if botUsers, err = appBotUsers(ctx, tx); err != nil {
			return store.Counts{}, err
		}
	}
	if err := tx.WipeWorkspace(ctx, keep...); err != nil {
		return store.Counts{}, err
	}
	if err := seed.Populate(ctx, tx, seed.Options{Seed: seedStr, Now: s.now()}, team); err != nil {
		return store.Counts{}, err
	}
	for _, u := range botUsers {
		if err := tx.InsertUser(ctx, u); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return store.Counts{}, fmt.Errorf("bot user %s: %w", u.ID, ErrResetIDCollision)
			}
			return store.Counts{}, err
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
