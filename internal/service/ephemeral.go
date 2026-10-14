package service

import (
	"context"
	"slices"
	"sync"
)

// EphemeralRingSize bounds how many ephemeral messages the service remembers.
const EphemeralRingSize = 1000

// EphemeralOpts describes a message only User sees.
type EphemeralOpts struct {
	Channel, User, ThreadTS string
	Content                 Content
}

// Ephemeral is a posted ephemeral message: visible to User only, never stored in history.
type Ephemeral struct {
	Channel, User, Sender, TS, ThreadTS string
	Content                             Content
}

// ephemerals is a bounded in-memory ring, oldest first. The zero value is ready.
type ephemerals struct {
	mu    sync.Mutex
	items []Ephemeral
}

func (r *ephemerals) add(e Ephemeral) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.items) == EphemeralRingSize {
		r.items = slices.Delete(r.items, 0, 1)
	}
	r.items = append(r.items, e)
}

func (r *ephemerals) all() []Ephemeral {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.items)
}

// PostEphemeral records a message only o.User can see and returns its ts. It is
// kept in the ring (see Ephemerals), never in history, and emits no events.
func (s *Service) PostEphemeral(ctx context.Context, sender User, o EphemeralOpts) (string, error) {
	c := o.Content
	if c.Text == "" && len(c.Blocks) == 0 && len(c.Attachments) == 0 {
		return "", ErrNoText
	}
	conv, err := s.st.Conversation(ctx, o.Channel)
	if err != nil {
		return "", notFoundAs(err, ErrChannelNotFound)
	}
	if err := s.checkWrite(ctx, sender, conv); err != nil {
		return "", err
	}
	members, err := s.st.MembersOf(ctx, []string{conv.ID})
	if err != nil {
		return "", err
	}
	if !slices.Contains(members[conv.ID], o.User) {
		return "", ErrUserNotInChannel
	}
	ts := s.ts.next(s.now())
	s.eph.add(Ephemeral{Channel: conv.ID, User: o.User, Sender: sender.ID, TS: ts, ThreadTS: o.ThreadTS, Content: c})
	return ts, nil
}

// Ephemerals returns the remembered ephemeral messages, oldest first.
func (s *Service) Ephemerals() []Ephemeral { return s.eph.all() }
