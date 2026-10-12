package server

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"localslack/internal/config"
	"localslack/internal/scenario"
	"localslack/internal/service"
	"localslack/internal/store"
)

// OpenStore opens and migrates the database, then populates it if it is empty
// (unless disabled): from cfg.Scenario when set, else the deterministic demo
// seed. A populated database is never touched; a scenario that cannot be
// applied for that reason is logged with the command that would apply it. A
// random seed is logged so a run can be reproduced.
func OpenStore(ctx context.Context, log *slog.Logger, cfg config.Config, now time.Time) (*store.Store, error) {
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if cfg.NoSeed {
		return st, nil
	}
	if err := populate(ctx, log, st, cfg, now); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func populate(ctx context.Context, log *slog.Logger, st *store.Store, cfg config.Config, now time.Time) error {
	in := service.SeedInput{Seed: cfg.Seed, Now: now}
	if in.Seed == "" {
		in.Seed = strconv.FormatUint(rand.Uint64(), 10)
	}
	if cfg.Scenario != "" {
		fx, err := scenario.LoadFile(cfg.Scenario)
		if err != nil {
			return fmt.Errorf("load scenario: %w", err)
		}
		in.Fixture = &fx
	}
	did, err := service.SeedEmpty(ctx, st, in)
	switch {
	case err != nil:
		return err
	case did && in.Fixture != nil:
		log.InfoContext(ctx, "seeded database from scenario", slog.String("scenario", cfg.Scenario), slog.String("seed", in.Seed))
	case did:
		log.InfoContext(ctx, "seeded database", slog.String("seed", in.Seed))
	case cfg.Scenario != "":
		log.WarnContext(ctx, "scenario ignored: database already has data",
			slog.String("scenario", cfg.Scenario), slog.String("fix", "localslack reset --scenario "+cfg.Scenario))
	}
	return nil
}
