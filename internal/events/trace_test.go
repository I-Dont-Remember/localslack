package events_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
)

func seqs(ds []events.Delivery) []int64 {
	out := make([]int64, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Seq)
	}
	return out
}

func addN(tr *events.Trace, n int) {
	for range n {
		tr.Add(events.Delivery{AppID: "A", EventType: "message", State: events.StatePending})
	}
}

func TestTraceListFilters(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(100)
	tr.Add(events.Delivery{AppID: "A1", EventType: "message", State: events.StateDelivered})
	tr.Add(events.Delivery{AppID: "A2", EventType: "message", State: events.StateFailed})
	tr.Add(events.Delivery{AppID: "A1", EventType: "app_mention", State: events.StateDelivered})

	tests := []struct {
		name string
		f    events.Filter
		want []int64
	}{
		{"all, oldest first", events.Filter{}, []int64{1, 2, 3}},
		{"by app", events.Filter{AppID: "A1"}, []int64{1, 3}},
		{"by type", events.Filter{EventType: "message"}, []int64{1, 2}},
		{"by state", events.Filter{State: events.StateFailed}, []int64{2}},
		{"since is exclusive", events.Filter{Since: 1}, []int64{2, 3}},
		{"limit", events.Filter{Limit: 2}, []int64{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, _ := tr.List(tt.f)
			if diff := cmp.Diff(tt.want, seqs(got)); diff != "" {
				t.Errorf("seqs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTraceNextSinceEchoesInputWhenEmpty(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	addN(tr, 2)
	if _, next := tr.List(events.Filter{Since: 7}); next != 7 {
		t.Errorf("next_since = %d, want input since 7", next)
	}
	if _, next := tr.List(events.Filter{}); next != 2 {
		t.Errorf("next_since = %d, want last seq 2", next)
	}
}

func TestTraceRingDropsOldest(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(3)
	addN(tr, 5)
	got, _ := tr.List(events.Filter{})
	if diff := cmp.Diff([]int64{3, 4, 5}, seqs(got)); diff != "" {
		t.Errorf("after wrap (-want +got):\n%s", diff)
	}
}

func TestTraceClearKeepsSeqMonotonic(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	addN(tr, 2)
	genBefore := tr.Generation()
	tr.Clear()
	if tr.Generation() == genBefore {
		t.Error("Clear did not bump generation")
	}
	d := tr.Add(events.Delivery{AppID: "A"})
	if d.Seq != 3 {
		t.Errorf("seq after clear = %d, want 3 (monotonic)", d.Seq)
	}
	if got, _ := tr.List(events.Filter{}); len(got) != 1 {
		t.Errorf("items after clear+add = %d, want 1", len(got))
	}
}

func TestTraceUpdateAndCopyIsolation(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	d := tr.Add(events.Delivery{AppID: "A", State: events.StatePending})
	tr.Update(d.ID, func(x *events.Delivery) {
		x.State = events.StateDelivered
		x.Attempts = append(x.Attempts, events.Attempt{N: 1, Status: 200})
	})
	got, ok := tr.Get(d.ID)
	if !ok || got.State != events.StateDelivered || len(got.Attempts) != 1 {
		t.Fatalf("Get = %+v, %v", got, ok)
	}
	got.Attempts[0].Status = 999 // mutating a copy must not leak back
	again, _ := tr.Get(d.ID)
	if again.Attempts[0].Status != 200 {
		t.Errorf("stored attempt status = %d, copy mutation leaked", again.Attempts[0].Status)
	}
	if _, ok := tr.Get("nope"); ok {
		t.Error("Get(unknown) ok = true")
	}
}

func TestTraceWaitWakesOnAdd(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	done := make(chan []events.Delivery, 1)
	go func() {
		got, _, _ := tr.Wait(t.Context(), events.Filter{}, 2, 5*time.Second)
		done <- got
	}()
	addN(tr, 2)
	select {
	case got := <-done:
		if len(got) != 2 {
			t.Errorf("Wait returned %d items, want 2", len(got))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not wake on Add")
	}
}

func TestTraceWaitWakesOnStateChange(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	d := tr.Add(events.Delivery{AppID: "A", State: events.StatePending})
	done := make(chan int, 1)
	go func() {
		got, _, _ := tr.Wait(t.Context(), events.Filter{State: events.StateDelivered}, 1, 5*time.Second)
		done <- len(got)
	}()
	tr.Update(d.ID, func(x *events.Delivery) { x.State = events.StateDelivered })
	select {
	case n := <-done:
		if n != 1 {
			t.Errorf("Wait returned %d items, want 1", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not wake on Update")
	}
}

func TestTraceWaitTimesOutWithWhatItHas(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	addN(tr, 1)
	got, _, _ := tr.Wait(t.Context(), events.Filter{}, 2, 30*time.Millisecond)
	if len(got) != 1 {
		t.Errorf("Wait returned %d items, want the 1 available", len(got))
	}
}

func TestTraceWaitWakesOnClearWithNewGeneration(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	type result struct {
		n   int
		gen int64
	}
	done := make(chan result, 1)
	start := tr.Generation()
	go func() {
		got, _, gen := tr.Wait(t.Context(), events.Filter{}, 1, 5*time.Second)
		done <- result{len(got), gen}
	}()
	// Let the waiter park; Clear must wake it even though min is unmet.
	deadline := time.After(5 * time.Second)
	for {
		tr.Clear()
		select {
		case r := <-done:
			if r.n != 0 || r.gen == start {
				t.Errorf("woken with n=%d gen=%d, want 0 items and a new generation", r.n, r.gen)
			}
			return
		case <-deadline:
			t.Fatal("Wait did not wake on Clear")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestTraceWaitHonorsContext(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	tr.Wait(ctx, events.Filter{}, 1, 5*time.Second)
	if time.Since(start) > time.Second {
		t.Error("Wait ignored a cancelled context")
	}
}

func TestTraceIDsAreUniqueAndStable(t *testing.T) {
	t.Parallel()

	tr := events.NewTrace(10)
	var ids []string
	for range 3 {
		ids = append(ids, tr.Add(events.Delivery{}).ID)
	}
	if len(slices.Compact(slices.Clone(ids))) != 3 {
		t.Errorf("ids not unique: %v", ids)
	}
}
