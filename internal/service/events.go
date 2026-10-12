package service

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"localslack/internal/store"
)

// Event is something that happened in the workspace. Events are self-contained:
// consumers (the event dispatcher, socket mode) never call back into the service.
type Event interface{ event() }

// ChannelInfo is the conversation an event happened in.
type ChannelInfo struct {
	ID, Name, Kind string
	IsPrivate      bool
}

// Scope is the context every event carries: where it happened, and which bot
// users are members there (the apps that may be told about it).
type Scope struct {
	Channel ChannelInfo
	Targets []string // member user ids that are bot users, sorted
}

// MessagePosted fires after a new message commits.
type MessagePosted struct {
	Msg Message
	Scope
}

// MessageChanged fires after a message edit commits; Prev is the message before.
type MessageChanged struct {
	Msg, Prev Message
	Scope
}

// MessageDeleted fires after a message delete commits; Msg is the deleted message.
type MessageDeleted struct {
	Msg Message
	Scope
}

// ReactionAdded fires after a reaction commits. Msg is the reacted-to message.
type ReactionAdded struct {
	User, Reaction string
	Msg            Message
	Scope
}

func (MessagePosted) event()  {}
func (MessageChanged) event() {}
func (MessageDeleted) event() {}
func (ReactionAdded) event()  {}

// bus fans events out to subscribers. Subscribers are called synchronously from
// the write path, under the store's write ordering, so they MUST be O(1) and
// non-blocking: hand the event to a queue; never do I/O, take locks held by
// writers, or call Subscribe/Service writes inline.
type bus struct {
	mu   sync.Mutex // serializes Subscribe/unsubscribe
	subs atomic.Pointer[[]subscriber]
	next int
}

type subscriber struct {
	id int
	fn func(Event)
}

func (b *bus) subscribe(fn func(Event)) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	id := b.next
	b.store(append(slices.Clone(b.load()), subscriber{id: id, fn: fn}))
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.store(slices.DeleteFunc(slices.Clone(b.load()), func(s subscriber) bool { return s.id == id }))
		})
	}
}

func (b *bus) load() []subscriber {
	if p := b.subs.Load(); p != nil {
		return *p
	}
	return nil
}

func (b *bus) store(s []subscriber) { b.subs.Store(&s) }

func (b *bus) publish(e Event) {
	for _, s := range b.load() {
		s.fn(e)
	}
}

// Subscribe registers fn for every event and returns an unsubscribe func.
// fn runs inline in the committing goroutine, in commit order; see bus for the
// rules. Seed, populate and reset never publish events.
func (s *Service) Subscribe(fn func(Event)) (unsubscribe func()) { return s.bus.subscribe(fn) }

// scopeOf builds the event scope for conv inside tx.
func scopeOf(ctx context.Context, tx *store.Tx, conv store.Conversation) (Scope, error) {
	targets, err := tx.BotMemberIDs(ctx, conv.ID)
	if err != nil {
		return Scope{}, err
	}
	return Scope{
		Channel: ChannelInfo{ID: conv.ID, Name: conv.Name, Kind: conv.Kind, IsPrivate: conv.IsPrivate},
		Targets: targets,
	}, nil
}

// publishAfterCommit queues e to publish once tx commits (never on rollback).
func publishAfterCommit(s *Service, tx *store.Tx, e Event) {
	tx.AfterCommit(func() { s.bus.publish(e) })
}
