package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReadPIDFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content *string
		want    int
		wantErr error
	}{
		{name: "missing", content: nil, wantErr: errNoPIDFile},
		{name: "valid", content: ptr("123\n"), want: 123},
		{name: "garbage", content: ptr("abc"), wantErr: errInvalidPIDFile},
		{name: "empty", content: ptr(""), wantErr: errInvalidPIDFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "x.pid")
			if tt.content != nil {
				if err := os.WriteFile(path, []byte(*tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readPIDFile(path)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("pid = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProcessAlive(t *testing.T) {
	t.Parallel()
	if !processAlive(os.Getpid()) {
		t.Error("own process reported dead")
	}
	cmd := exec.CommandContext(context.Background(), "true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if processAlive(cmd.Process.Pid) {
		t.Error("exited process reported alive")
	}
}

func TestWaitExit(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(context.Background(), "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if waitExit(cmd.Process.Pid, 50*time.Millisecond) {
		t.Error("waitExit returned true for running process")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if !waitExit(cmd.Process.Pid, time.Second) {
		t.Error("waitExit returned false for dead process")
	}
}

func ptr[T any](v T) *T { return &v }
