// Package config loads LocalSlack settings from the environment.
package config

import (
	"fmt"
	"path/filepath"
	"strconv"

	"localslack/internal/xdg"
)

// Config holds process-level settings.
type Config struct {
	Addr   string
	DBPath string
	// Seed is the RNG seed for generated data; empty means pick one at random.
	Seed string
	// NoSeed skips seeding an empty database.
	NoSeed bool
}

// Load reads settings via getenv, so tests need not touch the process env.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:   getenv("LOCALSLACK_ADDR"),
		DBPath: getenv("LOCALSLACK_DB_PATH"),
		Seed:   getenv("LOCALSLACK_SEED"),
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:3232"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(xdg.New(getenv).AppDataDir(), "localslack.db")
	}
	if v := getenv("LOCALSLACK_NO_SEED"); v != "" {
		noSeed, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("LOCALSLACK_NO_SEED: %w", err)
		}
		cfg.NoSeed = noSeed
	}
	return cfg, nil
}
