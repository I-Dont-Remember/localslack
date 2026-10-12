package control_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/storetest"
)

type recorder struct {
	events.Noop
	mu    sync.Mutex
	calls []string
	// onReset runs inside Reset, i.e. while the dispatcher is paused.
	onReset func()
}

func (r *recorder) note(c string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *recorder) Pause()  { r.note("pause") }
func (r *recorder) Resume() { r.note("resume") }
func (r *recorder) Reset() {
	r.note("reset")
	if r.onReset != nil {
		r.onReset()
	}
}

func TestResetQuiescesDispatcherThenWipesThenResumes(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rec := &recorder{}
	var order []string
	hook := func() { order = append(order, "hook") }
	c := control.New(app.Service, rec, control.WithOnReset(hook))

	got, err := c.Reset(t.Context(), control.ResetOpts{Seed: "s1"})
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if diff := cmp.Diff([]string{"pause", "reset", "resume"}, rec.calls); diff != "" {
		t.Errorf("dispatcher calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"hook"}, order); diff != "" {
		t.Errorf("reset hooks (-want +got):\n%s", diff)
	}
	want := control.ResetResult{Mode: "partial", Generation: 1, Seed: "s1", Counts: got.Counts}
	got.TookMS = 0
	if diff := cmp.Diff(want, got); diff != "" || got.Counts.Users == 0 {
		t.Errorf("result (-want +got):\n%s", diff)
	}
}

func TestResetModeAndGeneration(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	c := control.New(app.Service, events.Noop{}, control.WithSeed("default-seed"))

	first, err1 := c.Reset(t.Context(), control.ResetOpts{})
	second, err2 := c.Reset(t.Context(), control.ResetOpts{All: true, Seed: "x"})

	if err1 != nil || err2 != nil {
		t.Fatalf("Reset errs: %v / %v", err1, err2)
	}
	if first.Mode != "partial" || first.Seed != "default-seed" || first.Generation != 1 {
		t.Errorf("first = %+v, want partial / default-seed / gen 1", first)
	}
	if second.Mode != "all" || second.Seed != "x" || second.Generation != 2 {
		t.Errorf("second = %+v, want all / x / gen 2", second)
	}
}

func TestResetResumesDispatcherOnFailure(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rec := &recorder{}
	c := control.New(app.Service, rec, control.WithOnReset(func() { t.Error("hooks must not run after a failed reset") }))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := c.Reset(ctx, control.ResetOpts{})

	if err == nil {
		t.Fatal("Reset with a cancelled context = nil error")
	}
	if diff := cmp.Diff([]string{"pause", "reset", "resume"}, rec.calls); diff != "" {
		t.Errorf("dispatcher calls (-want +got):\n%s", diff)
	}
}

func TestConcurrentResetIsRejected(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	entered, release := make(chan struct{}), make(chan struct{})
	rec := &recorder{onReset: func() { close(entered); <-release }}
	c := control.New(app.Service, rec)
	done := make(chan error, 1)
	go func() {
		_, err := c.Reset(t.Context(), control.ResetOpts{})
		done <- err
	}()
	<-entered

	_, err := c.Reset(t.Context(), control.ResetOpts{})
	close(release)

	if !errors.Is(err, control.ErrResetInProgress) {
		t.Errorf("concurrent Reset err = %v, want ErrResetInProgress", err)
	}
	if err := <-done; err != nil {
		t.Errorf("first Reset: %v", err)
	}
}

func TestSnapshotDelegatesToService(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	c := control.New(app.Service, events.Noop{})

	got, err := c.Snapshot(t.Context())
	want, werr := app.Service.Snapshot(t.Context())
	if err != nil || werr != nil {
		t.Fatalf("Snapshot err = %v / %v", err, werr)
	}
	if len(got) == 0 {
		t.Fatal("snapshot has no conversations")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("snapshot (-service +control):\n%s", diff)
	}
}
