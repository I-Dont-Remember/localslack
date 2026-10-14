package admin

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	// defaultPollTimeout applies when min is set without timeout_ms.
	defaultPollTimeout = 5 * time.Second
	// maxPollTimeout caps timeout_ms.
	maxPollTimeout = 30 * time.Second
)

// poll is the shared list/long-poll query: since (exclusive seq), limit, and
// wait up to Timeout for Min items.
type poll struct {
	Since, Limit, Min int
	Timeout           time.Duration
}

func parsePoll(v url.Values) (poll, error) {
	since, err1 := intParam(v, "since")
	limit, err2 := intParam(v, "limit")
	minN, err3 := intParam(v, "min")
	timeoutMS, err4 := intParam(v, "timeout_ms")
	if err := errors.Join(err1, err2, err3, err4); err != nil {
		return poll{}, err
	}
	p := poll{Since: since, Limit: limit, Min: minN, Timeout: time.Duration(timeoutMS) * time.Millisecond}
	if minN > 0 && v.Get("timeout_ms") == "" {
		p.Timeout = defaultPollTimeout
	}
	p.Timeout = min(p.Timeout, maxPollTimeout)
	return p, nil
}

// intParam parses an optional non-negative integer query parameter.
func intParam(v url.Values, key string) (int, error) {
	s := v.Get(key)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", key, s)
	}
	return n, nil
}
