package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"localslack/internal/store"
)

// TokenKind classifies a credential by what it can be.
type TokenKind string

// Token kinds.
const (
	TokenNone    TokenKind = ""        // no credential
	TokenBot     TokenKind = "bot"     // xoxb-
	TokenUser    TokenKind = "user"    // xoxp-<userID>
	TokenApp     TokenKind = "app"     // xapp-
	TokenInvalid TokenKind = "invalid" // unrecognised
)

// App is a registered Slack app. Stub: filled by the app-registry stream.
type App struct {
	ID, Name           string
	BotToken, AppToken string
	BotUserID, BotID   string
	Scopes             []string
	SocketMode         bool
}

// Principal is who a token acts as. User is zero when the token does not
// resolve (see Authenticate); App is nil for unregistered tokens.
type Principal struct {
	User      User
	App       *App
	TokenKind TokenKind
	Scopes    []string
}

// appCache maps tokens to registered apps. Replaced wholesale on any change.
type appCache struct {
	mu      sync.RWMutex
	byToken map[string]*App
}

func (c *appCache) set(apps []App) {
	m := make(map[string]*App, 2*len(apps))
	for i := range apps {
		a := apps[i]
		for _, t := range []string{a.BotToken, a.AppToken} {
			if t != "" {
				m[t] = &a
			}
		}
	}
	c.mu.Lock()
	c.byToken = m
	c.mu.Unlock()
}

func (c *appCache) get(token string) *App {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byToken[token]
}

// SetApps replaces the token->app cache. Call on register/delete/revoke/reset.
func (s *Service) SetApps(apps ...App) { s.apps.set(apps) }

// InvalidateApps empties the token->app cache.
func (s *Service) InvalidateApps() { s.apps.set(nil) }

// Identify classifies token and resolves what it can. It never fails: a bad or
// unresolvable token yields a Principal with a zero User. Use Authenticate to enforce.
func (s *Service) Identify(ctx context.Context, token string) Principal {
	p, _ := s.resolve(ctx, token)
	return p
}

// Authenticate resolves token to a Principal with a real User, or a domain error.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	p, err := s.resolve(ctx, token)
	if err != nil {
		return Principal{}, err
	}
	return p, nil
}

// DefaultBot is the principal of the built-in bot, for server-internal callers.
func (s *Service) DefaultBot(ctx context.Context) (Principal, error) {
	u, err := s.lookupAuth(s.st.BotUser(ctx))
	return Principal{User: u, TokenKind: TokenBot}, err
}

func (s *Service) resolve(ctx context.Context, token string) (Principal, error) {
	switch {
	case token == "":
		return Principal{}, ErrNotAuthed
	case strings.HasPrefix(token, "xoxb-"):
		return s.resolveBot(ctx, token)
	case strings.HasPrefix(token, "xoxp-"):
		u, err := s.lookupAuth(s.st.User(ctx, strings.TrimPrefix(token, "xoxp-")))
		p := Principal{TokenKind: TokenUser}
		if err == nil {
			p.User = u
		}
		return p, err
	case strings.HasPrefix(token, "xapp-"):
		if app := s.apps.get(token); app != nil {
			return Principal{App: app, TokenKind: TokenApp, Scopes: app.Scopes}, nil
		}
		return Principal{TokenKind: TokenApp}, ErrInvalidAuth
	}
	return Principal{TokenKind: TokenInvalid}, ErrInvalidAuth
}

// resolveBot maps a registered app's bot token to its bot; any other xoxb- is the default bot.
func (s *Service) resolveBot(ctx context.Context, token string) (Principal, error) {
	app := s.apps.get(token)
	if app == nil {
		return s.DefaultBot(ctx)
	}
	u, err := s.lookupAuth(s.st.User(ctx, app.BotUserID))
	p := Principal{App: app, TokenKind: TokenBot, Scopes: app.Scopes}
	if err == nil {
		p.User = u
	}
	return p, err
}

func (s *Service) lookupAuth(u User, err error) (User, error) {
	if errors.Is(err, store.ErrNotFound) {
		return User{}, ErrInvalidAuth
	}
	return u, err
}
