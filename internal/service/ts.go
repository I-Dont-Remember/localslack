package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tsGen hands out strictly increasing Slack timestamps ("seconds.micros").
// Seeded from the greatest stored ts so restarts and clock skew never reuse one.
type tsGen struct {
	mu   sync.Mutex
	last int64 // microseconds since epoch
}

func newTSGen(maxStored string) (*tsGen, error) {
	g := &tsGen{}
	if maxStored == "" {
		return g, nil
	}
	us, err := parseTS(maxStored)
	if err != nil {
		return nil, err
	}
	g.last = us
	return g, nil
}

func (g *tsGen) next(now time.Time) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	us := max(now.UnixMicro(), g.last+1)
	g.last = us
	return fmt.Sprintf("%d.%06d", us/1_000_000, us%1_000_000)
}

// advanceTo raises the high-water mark to ts ("" is a no-op). Forward-only, so a
// reset can never make a later ts sort before one already handed out.
func (g *tsGen) advanceTo(ts string) error {
	if ts == "" {
		return nil
	}
	us, err := parseTS(ts)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.last = max(g.last, us)
	return nil
}

func parseTS(ts string) (int64, error) {
	sec, micro, ok := strings.Cut(ts, ".")
	if !ok {
		return 0, fmt.Errorf("malformed ts %q", ts)
	}
	s, err1 := strconv.ParseInt(sec, 10, 64)
	m, err2 := strconv.ParseInt(micro, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("malformed ts %q", ts)
	}
	return s*1_000_000 + m, nil
}
