package events

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"localslack/internal/constants"
)

const (
	userAgent       = "Slackbot 1.0 (+https://api.slack.com/robots)"
	maxResponseBody = 2048
)

// Retry reasons sent in X-Slack-Retry-Reason (the names Slack uses).
const (
	ReasonTimeout    = "http_timeout"
	ReasonConnection = "connection_failed"
	ReasonHTTPError  = "http_error"
	ReasonRedirects  = "too_many_redirects"
	ReasonUnknown    = "unknown_error"
)

// HTTPTransport POSTs signed events to a request URL. It never follows
// redirects and ignores proxy environment variables (events go to local apps).
type HTTPTransport struct {
	client *http.Client
	clock  Clock
}

// NewHTTPTransport returns an HTTPTransport that signs with clock's time.
func NewHTTPTransport(clock Clock) *HTTPTransport {
	return &HTTPTransport{
		clock: clock,
		client: &http.Client{
			Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Deliver implements Transport. The deadline comes from ctx.
func (t *HTTPTransport) Deliver(ctx context.Context, d Delivery) Attempt {
	mode := d.Spec.Mode
	if mode == "" {
		mode = SigValid
	}
	ts, sig := SignRequest(d.Spec.Secret, mode, d.Spec.StaleSeconds, t.clock.Now(), d.Payload)
	a := Attempt{SignatureMode: mode, Timestamp: ts, Signature: sig, RetryReason: d.Spec.RetryReason}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.Spec.URL, bytes.NewReader(d.Payload))
	if err != nil {
		a.Error, a.FailReason = err.Error(), ReasonUnknown
		return a
	}
	req.Header.Set("Content-Type", constants.ContentTypeJSON)
	req.Header.Set("User-Agent", userAgent)
	if ts != "" {
		req.Header.Set("X-Slack-Request-Timestamp", ts)
	}
	if sig != "" {
		req.Header.Set("X-Slack-Signature", sig)
	}
	if d.Spec.RetryNum > 0 {
		req.Header.Set("X-Slack-Retry-Num", strconv.Itoa(d.Spec.RetryNum))
		req.Header.Set("X-Slack-Retry-Reason", d.Spec.RetryReason)
	}

	start := time.Now()
	resp, err := t.client.Do(req)
	a.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		a.Error, a.FailReason = classify(err)
		return a
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	a.Status, a.ResponseBody = resp.StatusCode, string(body)
	a.NoRetry = resp.Header.Get("X-Slack-No-Retry") == "1"
	switch {
	case a.Status >= 200 && a.Status < 300:
	case a.Status >= 300 && a.Status < 400:
		a.Error, a.FailReason = fmt.Sprintf("redirect not followed (HTTP %d)", a.Status), ReasonRedirects
	default:
		a.Error, a.FailReason = fmt.Sprintf("unexpected HTTP status %d", a.Status), ReasonHTTPError
	}
	return a
}

func classify(err error) (msg, reason string) {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return "timeout", ReasonTimeout
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", ReasonUnknown
	}
	return err.Error(), ReasonConnection
}
