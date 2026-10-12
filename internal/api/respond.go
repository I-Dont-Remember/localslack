package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"localslack/internal/calllog"
	"localslack/internal/constants"
)

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
	w.Header().Set("Content-Type", constants.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.log.ErrorContext(ctx, "encode response", slog.Any("error", err))
	}
}

// capture tees the status and body so the Recorder can see what was sent.
type capture struct {
	http.ResponseWriter
	status   int
	buf      bytes.Buffer
	overflow bool
}

func newCapture(w http.ResponseWriter) *capture {
	return &capture{ResponseWriter: w, status: http.StatusOK}
}

func (c *capture) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *capture) Write(b []byte) (int, error) {
	if c.buf.Len()+len(b) > calllog.MaxResponseBytes {
		c.overflow = true
	} else {
		c.buf.Write(b)
	}
	return c.ResponseWriter.Write(b)
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
