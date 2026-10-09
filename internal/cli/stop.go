package cli

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"localslack/internal/xdg"
)

const stopTimeout = 5 * time.Second

// StopCmd stops the background LocalSlack server.
type StopCmd struct{}

// Run stops the background server by sending SIGTERM to the stored PID and waiting for exit.
func (c *StopCmd) Run() error {
	pidPath := xdg.PIDFile()
	pid, err := readPIDFile(pidPath)
	if errors.Is(err, errNoPIDFile) {
		return fmt.Errorf("server not running (no PID file at %s)", pidPath)
	}
	if err != nil {
		return err
	}
	if !processAlive(pid) {
		if removeErr := os.Remove(pidPath); removeErr != nil {
			return fmt.Errorf("remove stale PID file: %w", removeErr)
		}
		return fmt.Errorf("server process %d not found (stale PID file removed)", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	if !waitExit(pid, stopTimeout) {
		return fmt.Errorf("process %d still running %s after SIGTERM", pid, stopTimeout)
	}
	if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove PID file: %w", err)
	}
	fmt.Printf("LocalSlack stopped (PID %d)\n", pid)
	return nil
}
