// Package cli implements the command-line interface commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"localslack/internal/server"
	"localslack/internal/xdg"
)

// StartCmd starts the LocalSlack server.
type StartCmd struct {
	Addr   string `help:"Listen address." default:":3232" env:"LOCALSLACK_ADDR"`
	Detach bool   `short:"d" help:"Run server in the background."`
}

// Run starts the server, or detaches it to the background if -d is set.
func (c *StartCmd) Run() error {
	if c.Detach {
		return c.startDetached()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return server.Run(ctx, c.Addr)
}

func (c *StartCmd) startDetached() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	cmd := exec.CommandContext(context.Background(), exe, "start", "--addr", c.Addr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background server: %w", err)
	}
	pidPath := xdg.PIDFile()
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	fmt.Printf("LocalSlack started (PID %d)\n", cmd.Process.Pid)
	return nil
}
