package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	errNoPIDFile      = errors.New("no PID file")
	errInvalidPIDFile = errors.New("invalid PID file")
)

// readPIDFile returns the PID stored at path.
func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, errNoPIDFile
		}
		return 0, fmt.Errorf("read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("%w: %q", errInvalidPIDFile, strings.TrimSpace(string(data)))
	}
	return pid, nil
}

// processAlive reports whether a process with pid exists (signal 0 probe).
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// waitExit polls until pid is gone or timeout elapses; reports whether it exited.
func waitExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
