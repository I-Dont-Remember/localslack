package control_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/storetest"
)

type recorder struct {
	events.Noop
	calls []string
}

func (r *recorder) Pause()  { r.calls = append(r.calls, "pause") }
func (r *recorder) Resume() { r.calls = append(r.calls, "resume") }

func TestResetQuiescesDispatcherAndAlwaysResumes(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rec := &recorder{}
	c := control.New(app.Service, rec)

	err := c.Reset(t.Context(), control.ResetOpts{})

	if !errors.Is(err, control.ErrNotImplemented) {
		t.Errorf("Reset err = %v, want ErrNotImplemented until the reset stream lands", err)
	}
	if diff := cmp.Diff([]string{"pause", "resume"}, rec.calls); diff != "" {
		t.Errorf("dispatcher calls (-want +got):\n%s", diff)
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
