package control

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"localslack/internal/scenario"
	"localslack/internal/service"
)

// MaxSnapshots caps how many named snapshots are kept in memory.
const MaxSnapshots = 50

// CurrentRef names the live state in Diff.
const CurrentRef = "current"

var (
	// ErrInvalidSnapshotName: names are 1-64 of letters, digits, '.', '_' and '-', not starting with '.', '_' or '-', and never "current".
	ErrInvalidSnapshotName = errors.New("control: invalid snapshot name")
	// ErrSnapshotExists is returned when a snapshot name is taken.
	ErrSnapshotExists = errors.New("control: snapshot already exists")
	// ErrSnapshotLimit is returned when MaxSnapshots snapshots exist.
	ErrSnapshotLimit = errors.New("control: too many snapshots")
	// ErrSnapshotNotFound is returned for an unknown snapshot name.
	ErrSnapshotNotFound = errors.New("control: snapshot not found")

	snapshotNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// SnapshotCounts sizes a snapshot.
type SnapshotCounts struct{ Users, Channels, DMs, Messages, Reactions int }

// SnapshotInfo describes a stored snapshot.
type SnapshotInfo struct {
	Name    string
	Created time.Time
	Counts  SnapshotCounts
}

type snapshot struct {
	info SnapshotInfo
	fx   scenario.Fixture
}

// snapshots is the volatile, in-memory snapshot store. Fixtures are immutable
// once stored: nothing mutates them, so readers share them freely.
type snapshots struct {
	mu    sync.Mutex
	items []snapshot // creation order
}

func (s *snapshots) add(sn snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.indexLocked(sn.info.Name) >= 0 {
		return ErrSnapshotExists
	}
	if len(s.items) >= MaxSnapshots {
		return ErrSnapshotLimit
	}
	s.items = append(s.items, sn)
	return nil
}

func (s *snapshots) indexLocked(name string) int {
	return slices.IndexFunc(s.items, func(x snapshot) bool { return x.info.Name == name })
}

func (s *snapshots) has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.indexLocked(name) >= 0
}

func (s *snapshots) get(name string) (snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexLocked(name); i >= 0 {
		return s.items[i], nil
	}
	return snapshot{}, fmt.Errorf("%w: %q", ErrSnapshotNotFound, name)
}

func (s *snapshots) list() []SnapshotInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SnapshotInfo, len(s.items))
	for i, x := range s.items {
		out[i] = x.info
	}
	return out
}

func (s *snapshots) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
}

// CreateSnapshot stores the current state (one consistent read) under name.
func (c *Control) CreateSnapshot(ctx context.Context, name string) (SnapshotInfo, error) {
	if !snapshotNameRE.MatchString(name) || strings.EqualFold(name, CurrentRef) {
		return SnapshotInfo{}, ErrInvalidSnapshotName
	}
	if c.snaps.has(name) {
		return SnapshotInfo{}, ErrSnapshotExists
	}
	fx, err := c.svc.Export(ctx)
	if err != nil {
		return SnapshotInfo{}, err
	}
	sn := snapshot{info: SnapshotInfo{Name: name, Created: c.now().UTC(), Counts: countsOf(fx)}, fx: fx}
	if err := c.snaps.add(sn); err != nil {
		return SnapshotInfo{}, err
	}
	return sn.info, nil
}

func countsOf(fx scenario.Fixture) SnapshotCounts {
	n := SnapshotCounts{Users: len(fx.Users), Channels: len(fx.Channels), DMs: len(fx.DMs), Messages: len(fx.Messages)}
	for _, m := range fx.Messages {
		for _, r := range m.Reactions {
			n.Reactions += len(r.Users)
		}
	}
	return n
}

// Snapshots lists stored snapshots, oldest first.
func (c *Control) Snapshots() []SnapshotInfo { return c.snaps.list() }

// SnapshotFixture returns a stored snapshot as a fixture (usable as a scenario).
// The result is shared: do not modify it.
func (c *Control) SnapshotFixture(name string) (scenario.Fixture, error) {
	sn, err := c.snaps.get(name)
	return sn.fx, err
}

// State returns the live workspace as a fixture.
func (c *Control) State(ctx context.Context) (scenario.Fixture, error) { return c.svc.Export(ctx) }

// RestoreResult reports a completed restore.
type RestoreResult struct {
	Name   string
	Counts service.ResetCounts
	TookMS int64
}

// Restore replaces the workspace with a snapshot, with the dispatcher paused:
// like a partial reset, apps and their bot users are kept and rejoined to public
// channels, and no events are emitted. Unlike Reset it leaves the call log,
// deliveries, faults and other snapshots alone, so a test can inspect what led
// up to the restore. Fails with ErrResetInProgress while a reset or restore runs.
func (c *Control) Restore(ctx context.Context, name string) (RestoreResult, error) {
	sn, err := c.snaps.get(name)
	if err != nil {
		return RestoreResult{}, err
	}
	if !c.resetMu.TryLock() {
		return RestoreResult{}, ErrResetInProgress
	}
	defer c.resetMu.Unlock()
	start := c.now()
	c.disp.Pause()
	defer c.disp.Resume()
	counts, err := c.svc.Restore(ctx, sn.fx)
	if err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Name: name, Counts: counts, TookMS: c.now().Sub(start).Milliseconds()}, nil
}

// Diff compares two states, each a snapshot name or CurrentRef.
func (c *Control) Diff(ctx context.Context, from, to string) (scenario.Delta, error) {
	a, err := c.resolve(ctx, from)
	if err != nil {
		return scenario.Delta{}, err
	}
	b, err := c.resolve(ctx, to)
	if err != nil {
		return scenario.Delta{}, err
	}
	return scenario.Diff(a, b), nil
}

func (c *Control) resolve(ctx context.Context, ref string) (scenario.Fixture, error) {
	if ref == CurrentRef {
		return c.svc.Export(ctx)
	}
	return c.SnapshotFixture(ref)
}
