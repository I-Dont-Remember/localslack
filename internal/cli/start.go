// Package cli implements the command-line interface commands.
package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"localslack/internal/server"
)

// StartCmd starts the local Slack API server.
type StartCmd struct {
	Addr string `help:"Listen address." default:":3232" env:"LOCALSLACK_ADDR"`
}

// Run starts the server and blocks until interrupted.
func (c *StartCmd) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return server.Run(ctx, c.Addr)
}
