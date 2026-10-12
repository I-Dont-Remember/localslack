package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"localslack/internal/config"
	"localslack/internal/constants"
)

const resetTimeout = 30 * time.Second

// ResetCmd asks a running server to reset its state (POST /_localslack/reset).
type ResetCmd struct {
	Addr     string `help:"Server address (default 127.0.0.1:3232)." env:"LOCALSLACK_ADDR"`
	All      bool   `help:"Also remove apps (blank server). Default keeps apps."`
	Seed     string `help:"Re-seed generated data with this seed."`
	Scenario string `help:"Scenario fixture file (.json, .toml, .yaml) to populate instead of the demo data."`
}

// Run resets the server and prints its JSON reply.
func (c *ResetCmd) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), resetTimeout)
	defer cancel()
	return c.run(ctx, os.Stdout)
}

func (c *ResetCmd) run(ctx context.Context, out io.Writer) error {
	addr := c.Addr
	if addr == "" {
		cfg, err := config.Load(os.Getenv)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		addr = cfg.Addr
	}
	body := map[string]any{}
	if c.Seed != "" {
		body["seed"] = c.Seed
	}
	if c.Scenario != "" {
		// The server reads the file, so the path must not depend on this process's working directory.
		path, err := filepath.Abs(c.Scenario)
		if err != nil {
			return fmt.Errorf("scenario path: %w", err)
		}
		body["scenario"] = path
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode reset request: %w", err)
	}
	url := "http://" + addr + "/_localslack/reset"
	if c.All {
		url += "?all=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build reset request: %w", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reach server at %s (is it running?): %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read reset reply: %w", err)
	}
	return report(out, resp.StatusCode, reply)
}

func report(out io.Writer, status int, reply []byte) error {
	switch {
	case status == http.StatusNotFound:
		return errors.New("reset not supported by this server (POST /_localslack/reset returned 404)")
	case status >= http.StatusMultipleChoices:
		var e struct {
			Error struct{ Code, Message string }
		}
		if json.Unmarshal(reply, &e) == nil && e.Error.Code != "" {
			return fmt.Errorf("reset failed: %s: %s (HTTP %d)", e.Error.Code, e.Error.Message, status)
		}
		return fmt.Errorf("reset failed: HTTP %d", status)
	}
	_, err := fmt.Fprintln(out, string(bytes.TrimSpace(reply)))
	return err
}
