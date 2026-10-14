package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"localslack/internal/calllog"
)

// slackContentType is what Slack sends on every API response.
const slackContentType = "application/json; charset=utf-8"

// fail renders a domain error as Slack does: HTTP 200 with ok:false.
// Unmapped errors are real faults and return 500.
func (a *API) fail(ctx context.Context, w http.ResponseWriter, err error) {
	if code, ok := ErrorCode(err); ok {
		a.write(ctx, w, http.StatusOK, map[string]any{"ok": false, "error": code})
		return
	}
	a.log.ErrorContext(ctx, "api error", slog.Any("error", err))
	a.write(ctx, w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
}

func (a *API) write(ctx context.Context, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", slackContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}

// capture holds the status and body until flush, so the Recorder sees the call
// before the client gets its response (a client that returns from a call can
// immediately query the call log and find it).
type capture struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	buf         bytes.Buffer
	overflow    bool
}

func newCapture(w http.ResponseWriter) *capture {
	return &capture{ResponseWriter: w, status: http.StatusOK}
}

func (c *capture) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader, c.status = true, status
}

func (c *capture) Write(b []byte) (int, error) {
	c.wroteHeader = true
	c.buf.Write(b)
	c.overflow = c.buf.Len() > calllog.MaxResponseBytes
	return len(b), nil
}

// flush sends the held response to the client.
func (c *capture) flush() error {
	c.ResponseWriter.WriteHeader(c.status)
	_, err := c.buf.WriteTo(c.ResponseWriter)
	return err
}

// Unwrap lets http.ResponseController reach the real writer.
func (c *capture) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// body decodes the captured JSON object, or nil if oversized or not an object.
func (c *capture) body() map[string]any {
	if c.overflow {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(c.buf.Bytes(), &m) != nil {
		return nil
	}
	return m
}
