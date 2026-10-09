// Package service holds LocalSlack's rules: auth, naming, ts generation, threads, DMs.
// api and web depend on it; it alone depends on store.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"localslack/internal/ids"
	"localslack/internal/store"
)

// Domain types are the store's row types, re-exported so api/web never import store.
type (
	Team         = store.Team
	User         = store.User
	Conversation = store.Conversation
	Message      = store.Message
)

// Conversation kinds.
const (
	KindChannel = store.KindChannel
	KindIM      = store.KindIM
	KindMPIM    = store.KindMPIM
)

// Service implements LocalSlack's behavior over a store.
type Service struct {
	st  *store.Store
	now func() time.Time
	ids *ids.Generator
	ts  *tsGen
}

// New builds a Service. idSeed seeds id generation for newly created
// conversations; use a value distinct from the data seed.
func New(ctx context.Context, st *store.Store, now func() time.Time, idSeed string) (*Service, error) {
	maxTS, err := st.MaxTS(ctx)
	if err != nil {
		return nil, err
	}
	gen, err := newTSGen(maxTS)
	if err != nil {
		return nil, fmt.Errorf("seed ts generator: %w", err)
	}
	return &Service{st: st, now: now, ids: ids.NewGenerator(idSeed), ts: gen}, nil
}

// Authenticate resolves a token to the acting user: "xoxb-..." is the bot,
// "xoxp-<userID>" is that user.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	switch {
	case token == "":
		return User{}, ErrNotAuthed
	case strings.HasPrefix(token, "xoxb-"):
		return s.lookupAuth(s.st.BotUser(ctx))
	case strings.HasPrefix(token, "xoxp-"):
		return s.lookupAuth(s.st.User(ctx, strings.TrimPrefix(token, "xoxp-")))
	}
	return User{}, ErrInvalidAuth
}

func (s *Service) lookupAuth(u User, err error) (User, error) {
	if errors.Is(err, store.ErrNotFound) {
		return User{}, ErrInvalidAuth
	}
	return u, err
}

// Team returns the workspace.
func (s *Service) Team(ctx context.Context) (Team, error) { return s.st.Team(ctx) }

// Users returns all users.
func (s *Service) Users(ctx context.Context) ([]User, error) { return s.st.Users(ctx) }
