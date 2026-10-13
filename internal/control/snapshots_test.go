package control_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/scenario"
	"localslack/internal/storetest"
)

func newControl(t *testing.T, rec events.Dispatcher) (*control.Control, *storetest.App) {
	t.Helper()
	app := storetest.NewApp(t)
	if rec == nil {
		rec = events.Noop{}
	}
	return control.New(app.Service, rec), app
}

func TestSnapshotNameRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		snap    string
		wantErr error
	}{
		{"simple", "before-test", nil},
		{"dots and underscores", "v1.2_final", nil},
		{"max length", strings.Repeat("a", 64), nil},
		{"empty", "", control.ErrInvalidSnapshotName},
		{"too long", strings.Repeat("a", 65), control.ErrInvalidSnapshotName},
		{"slash", "a/b", control.ErrInvalidSnapshotName},
		{"space", "a b", control.ErrInvalidSnapshotName},
		{"leading dash", "-a", control.ErrInvalidSnapshotName},
		{"reserved current", "current", control.ErrInvalidSnapshotName},
		{"reserved current any case", "Current", control.ErrInvalidSnapshotName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newControl(t, nil)
			_, err := c.CreateSnapshot(t.Context(), tt.snap)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CreateSnapshot(%q) err = %v, want %v", tt.snap, err, tt.wantErr)
			}
		})
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	t.Parallel()

	c, app := newControl(t, nil)
	ctx := t.Context()
	want, err := app.Service.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	info, err := c.CreateSnapshot(ctx, "base")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if _, err := c.CreateSnapshot(ctx, "base"); !errors.Is(err, control.ErrSnapshotExists) {
		t.Errorf("duplicate err = %v, want ErrSnapshotExists", err)
	}
	if _, err := c.CreateSnapshot(ctx, "second"); err != nil {
		t.Fatalf("CreateSnapshot second: %v", err)
	}

	got, err := c.SnapshotFixture("base")
	if err != nil {
		t.Fatalf("SnapshotFixture: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("stored fixture (-export +snapshot):\n%s", diff)
	}
	if _, err := c.SnapshotFixture("nope"); !errors.Is(err, control.ErrSnapshotNotFound) {
		t.Errorf("unknown err = %v, want ErrSnapshotNotFound", err)
	}
	list := c.Snapshots()
	if len(list) != 2 || list[0].Name != "base" || list[1].Name != "second" {
		t.Fatalf("Snapshots = %+v, want base then second", list)
	}
	counts := list[0].Counts
	if counts.Users != len(want.Users) || counts.Channels != len(want.Channels) || counts.DMs != len(want.DMs) ||
		counts.Messages != len(want.Messages) || info.Counts != counts || info.Created.IsZero() {
		t.Errorf("counts = %+v (info %+v), want matching the export", counts, info)
	}
}

func TestSnapshotLimit(t *testing.T) {
	t.Parallel()

	c, _ := newControl(t, nil)
	for i := range control.MaxSnapshots {
		if _, err := c.CreateSnapshot(t.Context(), "s"+strconv.Itoa(i)); err != nil {
			t.Fatalf("CreateSnapshot %d: %v", i, err)
		}
	}
	if _, err := c.CreateSnapshot(t.Context(), "one-too-many"); !errors.Is(err, control.ErrSnapshotLimit) {
		t.Errorf("err = %v, want ErrSnapshotLimit", err)
	}
}

func TestRestoreReturnsToSnapshotWithoutEventsOrWipingLogs(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	hooked := false
	app := storetest.NewApp(t)
	c := control.New(app.Service, rec, control.WithOnReset(func() { hooked = true }))
	ctx := t.Context()
	if _, err := c.CreateSnapshot(ctx, "base"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	before, _ := app.Service.Export(ctx)
	caller := firstHumanUser(t, app)
	if _, err := app.Service.CreateChannel(ctx, caller, "scratch"); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	res, err := c.Restore(ctx, "base")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	after, _ := app.Service.Export(ctx)
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("state after restore (-snapshot +got):\n%s", diff)
	}
	if res.Name != "base" || res.Counts.Users != len(before.Users) {
		t.Errorf("result = %+v", res)
	}
	if hooked {
		t.Error("restore ran the reset hooks (call log etc. must survive a restore)")
	}
	if strings.Join(rec.calls, ",") != "pause,resume" {
		t.Errorf("dispatcher calls = %v, want pause,resume", rec.calls)
	}
	if _, err := c.Restore(ctx, "ghost"); !errors.Is(err, control.ErrSnapshotNotFound) {
		t.Errorf("Restore(ghost) err = %v, want ErrSnapshotNotFound", err)
	}
	if _, err := c.SnapshotFixture("base"); err != nil {
		t.Errorf("snapshot gone after restore: %v", err)
	}
}

func TestResetClearsSnapshots(t *testing.T) {
	t.Parallel()

	c, _ := newControl(t, nil)
	if _, err := c.CreateSnapshot(t.Context(), "base"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	if _, err := c.Reset(t.Context(), control.ResetOpts{Seed: "x"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if got := c.Snapshots(); len(got) != 0 {
		t.Errorf("snapshots after reset = %+v, want none", got)
	}
}

func TestRestoreAndResetExcludeEachOther(t *testing.T) {
	t.Parallel()

	entered, release := make(chan struct{}), make(chan struct{})
	rec := &recorder{onReset: func() { close(entered); <-release }}
	c, _ := newControl(t, rec)
	if _, err := c.CreateSnapshot(t.Context(), "base"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := c.Reset(t.Context(), control.ResetOpts{})
		done <- err
	}()
	<-entered

	_, err := c.Restore(t.Context(), "base")
	close(release)

	if !errors.Is(err, control.ErrResetInProgress) {
		t.Errorf("Restore during Reset err = %v, want ErrResetInProgress", err)
	}
	if err := <-done; err != nil {
		t.Errorf("Reset: %v", err)
	}
}

func TestResetWithScenario(t *testing.T) {
	t.Parallel()

	c, app := newControl(t, nil)
	fx := scenario.Fixture{
		Users:    []scenario.User{{ID: "UONE", Name: "one"}},
		Channels: []scenario.Channel{{ID: "CGEN", Name: "general", Members: []string{"UONE"}}},
	}

	res, err := c.Reset(t.Context(), control.ResetOpts{Seed: "sc", Scenario: &fx})
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	got, _ := app.Service.Export(t.Context())
	if len(got.Users) != 1 || got.Users[0].ID != "UONE" || res.Counts.Users != 1 || res.Counts.Channels != 1 {
		t.Errorf("state = %+v / counts %+v, want the scenario", got.Users, res.Counts)
	}
}

func TestDiffAndState(t *testing.T) {
	t.Parallel()

	c, app := newControl(t, nil)
	ctx := t.Context()
	if _, err := c.CreateSnapshot(ctx, "base"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	caller := firstHumanUser(t, app)
	ch, err := app.Service.CreateChannel(ctx, caller, "later")
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	d, err := c.Diff(ctx, "base", "current")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(d.Conversations.Added) != 1 || d.Conversations.Added[0].ID != ch.ID {
		t.Errorf("added conversations = %+v, want the new channel", d.Conversations.Added)
	}
	same, err := c.Diff(ctx, "base", "base")
	if err != nil || len(same.Conversations.Added)+len(same.Messages.Added) != 0 {
		t.Errorf("Diff(base,base) = %+v, %v; want empty", same, err)
	}
	if _, err := c.Diff(ctx, "base", "ghost"); !errors.Is(err, control.ErrSnapshotNotFound) || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("Diff unknown err = %v, want ErrSnapshotNotFound naming ghost", err)
	}
	cur, err := c.State(ctx)
	if err != nil || len(cur.Channels) == 0 {
		t.Errorf("State = %+v, %v", cur, err)
	}
}

func TestSnapshotCreatedUsesInjectedClock(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	c := control.New(app.Service, events.Noop{}, control.WithClock(func() time.Time { return at }))

	info, err := c.CreateSnapshot(t.Context(), "timed")
	if err != nil || !info.Created.Equal(at) {
		t.Errorf("info = %+v, %v; want created %v", info, err, at)
	}
}
