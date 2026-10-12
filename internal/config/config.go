// Package config loads LocalSlack settings from the environment.
package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	// AppManifest lists app manifest files (JSON/YAML) to register at startup.
	AppManifest []string
	// EventTimeout bounds one event delivery attempt.
	EventTimeout time.Duration
	// EventRetryDelays are the waits before each retry; empty disables retries.
	EventRetryDelays []time.Duration
	// Scenario is a fixture file to populate instead of the default seed.
	Scenario string
	// CallLogSize and DeliveryLogSize cap the in-memory ring buffers.
	CallLogSize, DeliveryLogSize int
	// StrictTokens rejects xoxb- tokens that belong to no registered app.
	StrictTokens bool
}

// Defaults for the test-loop settings.
const (
	DefaultEventTimeout     = 3 * time.Second
	DefaultEventRetryDelays = "0s,1m,5m"
	DefaultCallLogSize      = 10000
	DefaultDeliveryLogSize  = 2000
)

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
	if err := loadTestLoop(getenv, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadTestLoop(getenv func(string) string, cfg *Config) error {
	cfg.AppManifest = SplitList(getenv("LOCALSLACK_APP_MANIFEST"))
	cfg.Scenario = getenv("LOCALSLACK_SCENARIO")
	var err error
	if cfg.EventTimeout, err = envDuration(getenv, "LOCALSLACK_EVENT_TIMEOUT", DefaultEventTimeout); err != nil {
		return err
	}
	delays := getenv("LOCALSLACK_EVENT_RETRY_DELAYS")
	if delays == "" {
		delays = DefaultEventRetryDelays
	}
	if cfg.EventRetryDelays, err = ParseDelays(delays); err != nil {
		return fmt.Errorf("LOCALSLACK_EVENT_RETRY_DELAYS: %w", err)
	}
	if cfg.CallLogSize, err = envSize(getenv, "LOCALSLACK_CALL_LOG_SIZE", DefaultCallLogSize); err != nil {
		return err
	}
	if cfg.DeliveryLogSize, err = envSize(getenv, "LOCALSLACK_DELIVERY_LOG_SIZE", DefaultDeliveryLogSize); err != nil {
		return err
	}
	if v := getenv("LOCALSLACK_STRICT_TOKENS"); v != "" {
		if cfg.StrictTokens, err = strconv.ParseBool(v); err != nil {
			return fmt.Errorf("LOCALSLACK_STRICT_TOKENS: %w", err)
		}
	}
	return nil
}

// SplitList splits a comma-separated list, trimming blanks; empty input yields nil.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseDelays parses comma-separated durations ("0s,1m,5m"); "none" means no retries.
func ParseDelays(s string) ([]time.Duration, error) {
	if strings.TrimSpace(s) == "none" {
		return nil, nil
	}
	var out []time.Duration
	for _, p := range SplitList(s) {
		d, err := time.ParseDuration(p)
		if err != nil {
			return nil, err
		}
		if d < 0 {
			return nil, fmt.Errorf("negative delay %q", p)
		}
		out = append(out, d)
	}
	return out, nil
}

func envDuration(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func envSize(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s: want a positive integer, got %q", key, v)
	}
	return n, nil
}
