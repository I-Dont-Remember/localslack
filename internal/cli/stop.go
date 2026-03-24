package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"localslack/internal/xdg"
)

// StopCmd stops the background LocalSlack server.
type StopCmd struct{}

// Run stops the background server by sending SIGTERM to the stored PID.
func (c *StopCmd) Run() error {
	pidPath := xdg.PIDFile()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("server not running (no PID file at %s)", pidPath)
		}
		return fmt.Errorf("read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			if removeErr := os.Remove(pidPath); removeErr != nil {
				return fmt.Errorf("remove stale PID file: %w", removeErr)
			}
			return fmt.Errorf("server process %d not found (stale PID file removed)", pid)
		}
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	if err := os.Remove(pidPath); err != nil {
		return fmt.Errorf("remove PID file: %w", err)
	}
	fmt.Printf("LocalSlack stopped (PID %d)\n", pid)
	return nil
}
