package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"localslack/internal/config"
	"localslack/internal/constants"
)

const appsTimeout = 30 * time.Second

// AppsCmd manages apps on a running server (/_localslack/apps).
type AppsCmd struct {
	Add    AppsAddCmd    `cmd:"" help:"Register an app from a Slack manifest (JSON or YAML)."`
	List   AppsListCmd   `cmd:"" help:"List registered apps."`
	SetURL AppsSetURLCmd `cmd:"" name:"set-url" help:"Point an app's event delivery at a URL (its deliver_to)."`
	Env    AppsEnvCmd    `cmd:"" help:"Print an app's credentials as env vars for the app under test."`
}

// Conn says which server to talk to.
type Conn struct {
	Addr string `help:"Server address (default 127.0.0.1:3232)." env:"LOCALSLACK_ADDR"`
}

func (c Conn) base() (string, error) {
	addr := c.Addr
	if addr == "" {
		cfg, err := config.Load(os.Getenv)
		if err != nil {
			return "", fmt.Errorf("load config: %w", err)
		}
		addr = cfg.Addr
	}
	return "http://" + addr, nil
}

// call sends one request and returns the status and body; HTTP >= 400 is an error.
func (c Conn) call(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, appsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach server at %s (is it running?): %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read reply: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, apiError(resp.StatusCode, reply)
	}
	return reply, nil
}

func apiError(status int, reply []byte) error {
	var e struct {
		Error struct {
			Code, Message string
			Details       []struct{ Message, Pointer string }
		}
	}
	if json.Unmarshal(reply, &e) != nil || e.Error.Code == "" {
		return fmt.Errorf("HTTP %d", status)
	}
	msg := fmt.Sprintf("%s: %s (HTTP %d)", e.Error.Code, e.Error.Message, status)
	for _, d := range e.Error.Details {
		msg += fmt.Sprintf("\n  %s %s", d.Pointer, d.Message)
	}
	return fmt.Errorf("%s", msg)
}

// appInfo is the part of the app resource the CLI reads.
type appInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BotUserID   string `json:"bot_user_id"`
	Delivery    string `json:"delivery"`
	Verified    bool   `json:"verified"`
	DeliverTo   string `json:"deliver_to"`
	Credentials struct {
		SigningSecret string `json:"signing_secret"`
		BotToken      string `json:"bot_token"`
		AppToken      string `json:"app_token"`
	} `json:"credentials"`
	Subs struct {
		Effective string `json:"effective_request_url"`
	} `json:"event_subscriptions"`
	Warnings []string `json:"warnings"`
}

func (c Conn) apps(ctx context.Context) ([]appInfo, error) {
	reply, err := c.call(ctx, http.MethodGet, "/_localslack/apps", "", nil)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []appInfo `json:"items"`
	}
	if err := json.Unmarshal(reply, &list); err != nil {
		return nil, fmt.Errorf("decode apps: %w", err)
	}
	return list.Items, nil
}

// find resolves an app by id or (case-insensitive) name.
func (c Conn) find(ctx context.Context, ref string) (appInfo, error) {
	apps, err := c.apps(ctx)
	if err != nil {
		return appInfo{}, err
	}
	for _, a := range apps {
		if a.ID == ref || strings.EqualFold(a.Name, ref) {
			return a, nil
		}
	}
	return appInfo{}, fmt.Errorf("no app %q (see `localslack apps list`)", ref)
}

func printApp(out io.Writer, a appInfo) error {
	_, err := fmt.Fprintf(out, "%s  %s  bot=%s  delivery=%s  verified=%v\n  events -> %s\n", a.ID, a.Name, a.BotUserID, a.Delivery, a.Verified, orDash(a.Subs.Effective))
	if err != nil {
		return err
	}
	for _, w := range a.Warnings {
		if _, err := fmt.Fprintf(out, "  warning: %s\n", w); err != nil {
			return err
		}
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// AppsAddCmd registers an app.
type AppsAddCmd struct {
	Conn
	Manifest  string `arg:"" type:"existingfile" help:"Manifest file (.json, .yaml or .yml)."`
	DeliverTo string `name:"deliver-to" help:"Send this app's events to this URL (host-only keeps the manifest path)."`
	NoVerify  bool   `name:"no-verify" help:"Skip the url_verification handshake."`
}

// Run implements kong.
func (c *AppsAddCmd) Run() error { return c.run(context.Background(), os.Stdout) }

func (c *AppsAddCmd) run(ctx context.Context, out io.Writer) error {
	data, err := os.ReadFile(c.Manifest)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	ctype := constants.ContentTypeJSON
	if ext := strings.ToLower(filepath.Ext(c.Manifest)); ext == ".yaml" || ext == ".yml" {
		ctype = "application/yaml"
	}
	body := data
	if c.DeliverTo != "" || c.NoVerify {
		env := map[string]any{"manifest": string(data)}
		if c.DeliverTo != "" {
			env["deliver_to"] = c.DeliverTo
		}
		if c.NoVerify {
			env["verify"] = false
		}
		if body, err = json.Marshal(env); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		ctype = constants.ContentTypeJSON
	}
	reply, err := c.call(ctx, http.MethodPost, "/_localslack/apps", ctype, body)
	if err != nil {
		return err
	}
	var a appInfo
	if err := json.Unmarshal(reply, &a); err != nil {
		return fmt.Errorf("decode app: %w", err)
	}
	return printApp(out, a)
}

// AppsListCmd lists apps.
type AppsListCmd struct{ Conn }

// Run implements kong.
func (c *AppsListCmd) Run() error { return c.run(context.Background(), os.Stdout) }

func (c *AppsListCmd) run(ctx context.Context, out io.Writer) error {
	apps, err := c.apps(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tBOT USER\tDELIVERY\tVERIFIED\tEVENTS GO TO")
	for _, a := range apps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\t%s\n", a.ID, a.Name, a.BotUserID, a.Delivery, a.Verified, orDash(a.Subs.Effective))
	}
	return tw.Flush()
}

// AppsSetURLCmd sets an app's deliver_to.
type AppsSetURLCmd struct {
	Conn
	App string `arg:"" help:"App id or name."`
	URL string `arg:"" help:"Base URL such as http://localhost:3000, or a full URL with a path."`
}

// Run implements kong.
func (c *AppsSetURLCmd) Run() error { return c.run(context.Background(), os.Stdout) }

func (c *AppsSetURLCmd) run(ctx context.Context, out io.Writer) error {
	app, err := c.find(ctx, c.App)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"deliver_to": c.URL})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	reply, err := c.call(ctx, http.MethodPatch, "/_localslack/apps/"+app.ID, constants.ContentTypeJSON, body)
	if err != nil {
		return err
	}
	var a appInfo
	if err := json.Unmarshal(reply, &a); err != nil {
		return fmt.Errorf("decode app: %w", err)
	}
	return printApp(out, a)
}

// AppsEnvCmd prints an app's credentials as shell exports.
type AppsEnvCmd struct {
	Conn
	App string `arg:"" help:"App id or name."`
}

// Run implements kong.
func (c *AppsEnvCmd) Run() error { return c.run(context.Background(), os.Stdout) }

func (c *AppsEnvCmd) run(ctx context.Context, out io.Writer) error {
	app, err := c.find(ctx, c.App)
	if err != nil {
		return err
	}
	base, err := c.base()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "export SLACK_API_URL=%s/api/\nexport SLACK_APP_ID=%s\nexport SLACK_BOT_TOKEN=%s\nexport SLACK_SIGNING_SECRET=%s\nexport SLACK_APP_TOKEN=%s\n",
		base, app.ID, app.Credentials.BotToken, app.Credentials.SigningSecret, app.Credentials.AppToken)
	return err
}
