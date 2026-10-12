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
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
	"localslack/internal/xdg"
)

// StartCmd starts the LocalSlack server.
type StartCmd struct {
	Addr   string `help:"Listen address (default 127.0.0.1:3232; use 0.0.0.0:3232 to expose, e.g. in containers)." env:"LOCALSLACK_ADDR"`
	Detach bool   `short:"d" help:"Run server in the background."`

	AppManifest      []string      `name:"app-manifest" sep:"," help:"App manifest file (JSON/YAML) to register at startup; repeatable." env:"LOCALSLACK_APP_MANIFEST"`
	EventTimeout     time.Duration `help:"Per-attempt event delivery timeout (default 3s)." env:"LOCALSLACK_EVENT_TIMEOUT"`
	EventRetryDelays string        `help:"Comma-separated waits before each delivery retry (default 0s,1m,5m; 'none' disables)." env:"LOCALSLACK_EVENT_RETRY_DELAYS"`
	Scenario         string        `help:"Scenario fixture file to populate instead of the default seed." env:"LOCALSLACK_SCENARIO"`
	CallLogSize      int           `help:"Call log capacity (default 10000)." env:"LOCALSLACK_CALL_LOG_SIZE"`
	DeliveryLogSize  int           `help:"Delivery log capacity (default 2000)." env:"LOCALSLACK_DELIVERY_LOG_SIZE"`
	StrictTokens     bool          `help:"Reject bot tokens that belong to no registered app." env:"LOCALSLACK_STRICT_TOKENS"`
}

// apply overlays the flags that were set (which also read their env vars) onto cfg.
func (c *StartCmd) apply(cfg *config.Config) error {
	if c.Addr != "" {
		cfg.Addr = c.Addr
	}
	if len(c.AppManifest) > 0 {
		cfg.AppManifest = c.AppManifest
	}
	if c.EventTimeout > 0 {
		cfg.EventTimeout = c.EventTimeout
	}
	if c.EventRetryDelays != "" {
		delays, err := config.ParseDelays(c.EventRetryDelays)
		if err != nil {
			return fmt.Errorf("--event-retry-delays: %w", err)
		}
		cfg.EventRetryDelays = delays
	}
	if c.Scenario != "" {
		cfg.Scenario = c.Scenario
	}
	if c.CallLogSize > 0 {
		cfg.CallLogSize = c.CallLogSize
	}
	if c.DeliveryLogSize > 0 {
		cfg.DeliveryLogSize = c.DeliveryLogSize
	}
	cfg.StrictTokens = cfg.StrictTokens || c.StrictTokens
	return nil
}

// detachedArgs is the argv (minus the program) for the background child: every set flag.
func (c *StartCmd) detachedArgs() []string {
	args := []string{"start"}
	add := func(flag, val string) { args = append(args, flag, val) }
	if c.Addr != "" {
		add("--addr", c.Addr)
	}
	for _, m := range c.AppManifest {
		add("--app-manifest", m)
	}
	if c.EventTimeout > 0 {
		add("--event-timeout", c.EventTimeout.String())
	}
	if c.EventRetryDelays != "" {
		add("--event-retry-delays", c.EventRetryDelays)
	}
	if c.Scenario != "" {
		add("--scenario", c.Scenario)
	}
	if c.CallLogSize > 0 {
		add("--call-log-size", strconv.Itoa(c.CallLogSize))
	}
	if c.DeliveryLogSize > 0 {
		add("--delivery-log-size", strconv.Itoa(c.DeliveryLogSize))
	}
	if c.StrictTokens {
		args = append(args, "--strict-tokens")
	}
	return args
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
	if err := c.apply(&cfg); err != nil {
		return err
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
	cmd := exec.CommandContext(context.Background(), exe, c.detachedArgs()...)
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
