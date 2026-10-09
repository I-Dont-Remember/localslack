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

	"localslack/internal/config"
	"localslack/internal/server"
	"localslack/internal/xdg"
)

// StartCmd starts the LocalSlack server.
type StartCmd struct {
	Addr   string `help:"Listen address (default 127.0.0.1:3232; use 0.0.0.0:3232 to expose, e.g. in containers)." env:"LOCALSLACK_ADDR"`
	Detach bool   `short:"d" help:"Run server in the background."`
}

// Run starts the server, or detaches it to the background if -d is set.
func (c *StartCmd) Run() error {
	if c.Detach {
		return c.startDetached()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if c.Addr != "" { // flag (which also reads LOCALSLACK_ADDR) wins over the config default
		cfg.Addr = c.Addr
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return server.Run(ctx, log, cfg)
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
	args := []string{"start"}
	if c.Addr != "" {
		args = append(args, "--addr", c.Addr)
	}
	cmd := exec.CommandContext(context.Background(), exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background server: %w", err)
	}
	if err := os.WriteFile(xdg.PIDFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	addr := c.Addr
	if addr == "" {
		if cfg, err := config.Load(os.Getenv); err == nil {
			addr = cfg.Addr
		}
	}
	fmt.Printf("LocalSlack started (PID %d) at http://%s\n", cmd.Process.Pid, addr)
	return nil
}
