package server

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"localslack/internal/config"
	"localslack/internal/seed"
	"localslack/internal/store"
)

// OpenStore opens and migrates the database, then seeds it if it is empty
// (unless disabled). A random seed is logged so a run can be reproduced.
func OpenStore(ctx context.Context, log *slog.Logger, cfg config.Config, now time.Time) (*store.Store, error) {
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if cfg.NoSeed {
		return st, nil
	}
	seedStr := cfg.Seed
	if seedStr == "" {
		seedStr = strconv.FormatUint(rand.Uint64(), 10)
	}
	did, err := seed.Apply(ctx, st, seed.Options{Seed: seedStr, Now: now})
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	if did {
		log.InfoContext(ctx, "seeded database", slog.String("seed", seedStr))
	}
	return st, nil
}
