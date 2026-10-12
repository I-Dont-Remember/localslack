// Package control orchestrates cross-cutting operations (reset, snapshot) that
// span the service and the event dispatcher. admin and the UI both call it, so
// they behave identically. It must not import store, api or web.
package control

import (
	"context"
	"errors"

	"localslack/internal/events"
	"localslack/internal/service"
)

// ErrNotImplemented is returned by operations whose stream has not landed.
var ErrNotImplemented = errors.New("control: not implemented")

// Control runs orchestrated operations.
type Control struct {
	svc  *service.Service
	disp events.Dispatcher
}

// New returns a Control over svc and disp.
func New(svc *service.Service, disp events.Dispatcher) *Control {
	return &Control{svc: svc, disp: disp}
}

// ResetOpts selects what a reset repopulates; extended by the reset stream.
type ResetOpts struct {
	Seed string
}

// Reset wipes and repopulates state with the dispatcher quiesced. Shell: the
// reset stream adds the service wipe/populate between Pause and Reset.
func (c *Control) Reset(context.Context, ResetOpts) error {
	c.disp.Pause()
	defer c.disp.Resume()
	return ErrNotImplemented
}

// Snapshot returns every conversation with members and messages. Shell: the
// snapshot stream swaps this for the Fixture export.
func (c *Control) Snapshot(ctx context.Context) ([]service.ConversationState, error) {
	return c.svc.Snapshot(ctx)
}
