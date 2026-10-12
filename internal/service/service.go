// Package service holds LocalSlack's rules: auth, naming, ts generation, threads, DMs.
// api and web depend on it; it alone depends on store.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"localslack/internal/ids"
	"localslack/internal/store"
)

// Team and User are the store's plain rows, re-exported so api/web never import store.
// Conversation and Message are service-owned (see types.go).
type (
	Team = store.Team
	User = store.User
)

// Conversation kinds.
const (
	KindChannel = store.KindChannel
	KindIM      = store.KindIM
	KindMPIM    = store.KindMPIM
)

// Service implements LocalSlack's behavior over a store.
type Service struct {
	st   *store.Store
	now  func() time.Time
	ids  *ids.Generator
	ts   *tsGen
	apps appCache
}

// New builds a Service. idSeed makes new conversation ids deterministic for
// tests; production passes "" to get a fresh random seed per process, so ids
// never replay after a restart. Exactly one Service may write to a store.
func New(ctx context.Context, st *store.Store, now func() time.Time, idSeed string) (*Service, error) {
	maxTS, err := st.MaxTS(ctx)
	if err != nil {
		return nil, err
	}
	gen, err := newTSGen(maxTS)
	if err != nil {
		return nil, fmt.Errorf("seed ts generator: %w", err)
	}
	if idSeed == "" {
		if idSeed, err = randomSeed(); err != nil {
			return nil, err
		}
	}
	return &Service{st: st, now: now, ids: ids.NewGenerator(idSeed), ts: gen}, nil
}

// Team returns the workspace.
func (s *Service) Team(ctx context.Context) (Team, error) { return s.st.Team(ctx) }

// Users returns all users.
func (s *Service) Users(ctx context.Context) ([]User, error) { return s.st.Users(ctx) }

func randomSeed() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random id seed: %w", err)
	}
	return hex.EncodeToString(b), nil
}
