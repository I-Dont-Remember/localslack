// Package cli implements the command-line interface commands.
package cli

import (
	"context"
	"fmt"
	"log/slog"
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
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return server.Run(ctx, log, c.Addr)
}

func (c *StartCmd) startDetached() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	if pid, err := readPIDFile(xdg.PIDFile()); err == nil && processAlive(pid) {
		return fmt.Errorf("already running (PID %d); run `localslack stop` first", pid)
	}
	stateDir := xdg.AppStateDir()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(stateDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.CommandContext(context.Background(), exe, "start", "--addr", c.Addr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background server: %w", err)
	}
	if err := os.WriteFile(xdg.PIDFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	fmt.Printf("LocalSlack started (PID %d)\n", cmd.Process.Pid)
	return nil
}
