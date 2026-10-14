package calllog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/calllog"
)

func rec(method string) calllog.CallRecord { return calllog.CallRecord{Method: method, OK: true} }

func seqs(r calllog.Result) []int64 {
	var out []int64
	for _, c := range r.Calls {
		out = append(out, c.Seq)
	}
	return out
}

func TestRecordAssignsMonotonicSeq(t *testing.T) {
	t.Parallel()

	l := calllog.New(10)
	for range 3 {
		l.Record(rec("a"))
	}
	got := seqs(l.Query(t.Context(), calllog.Query{}))
	if diff := cmp.Diff([]int64{1, 2, 3}, got); diff != "" {
		t.Errorf("seqs (-want +got):\n%s", diff)
	}
}

func TestRingKeepsNewest(t *testing.T) {
	t.Parallel()

	l := calllog.New(3)
	for i := range 5 {
		l.Record(rec(fmt.Sprint("m", i)))
	}
	got := seqs(l.Query(t.Context(), calllog.Query{}))
	if diff := cmp.Diff([]int64{3, 4, 5}, got); diff != "" {
		t.Errorf("seqs after wrap (-want +got):\n%s", diff)
	}
}

func TestQueryFilters(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	l := calllog.New(10)
	l.Record(calllog.CallRecord{Method: "chat.postMessage", OK: true, Params: map[string]any{"channel": "C1"}, Actor: calllog.Actor{AppID: "A1"}})
	l.Record(calllog.CallRecord{Method: "chat.postMessage", OK: false, Error: "is_archived", Params: map[string]any{"channel": "C2"}, Actor: calllog.Actor{AppID: "A2"}})
	l.Record(calllog.CallRecord{Method: "auth.test", OK: true})
	tests := []struct {
		name string
		q    calllog.Query
		want []int64
	}{
		{"all", calllog.Query{}, []int64{1, 2, 3}},
		{"method", calllog.Query{Method: "chat.postMessage"}, []int64{1, 2}},
		{"channel", calllog.Query{Channel: "C2"}, []int64{2}},
		{"app", calllog.Query{AppID: "A1"}, []int64{1}},
		{"ok true", calllog.Query{OK: &yes}, []int64{1, 3}},
		{"ok false", calllog.Query{OK: &no}, []int64{2}},
		{"error", calllog.Query{Error: "is_archived"}, []int64{2}},
		{"since is exclusive", calllog.Query{Since: 1}, []int64{2, 3}},
		{"limit", calllog.Query{Limit: 2}, []int64{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := l.Query(t.Context(), tt.q)
			if diff := cmp.Diff(tt.want, seqs(res)); diff != "" {
				t.Errorf("seqs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNextSinceAndTruncation(t *testing.T) {
	t.Parallel()

	l := calllog.New(10)
	for range 3 {
		l.Record(rec("a"))
	}
	res := l.Query(t.Context(), calllog.Query{Limit: 2})
	if res.NextSince != 2 || !res.Truncated {
		t.Errorf("limited query: NextSince=%d Truncated=%v, want 2 true", res.NextSince, res.Truncated)
	}
	empty := l.Query(t.Context(), calllog.Query{Since: 3})
	if empty.NextSince != 3 || empty.Truncated || len(empty.Calls) != 0 {
		t.Errorf("empty query = %+v, want NextSince echoing since=3", empty)
	}
}

func TestRedactsSecretsWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	secret := "tok" + "en-value"
	params := map[string]any{
		"token": secret, "client_secret": "s", "channel": "C1",
		"nested": map[string]any{"Authorization": "Bearer x", "keep": "ok"},
	}
	l := calllog.New(5)
	l.Record(calllog.CallRecord{Method: "a", Params: params, Response: map[string]any{"signing_secret": "z", "ok": true}})

	got := l.Query(t.Context(), calllog.Query{}).Calls[0]
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "Bearer x") || strings.Contains(string(b), `"z"`) {
		t.Errorf("record leaked a secret: %s", b)
	}
	want := map[string]any{
		"token": "[redacted]", "client_secret": "[redacted]", "channel": "C1",
		"nested": map[string]any{"Authorization": "[redacted]", "keep": "ok"},
	}
	if diff := cmp.Diff(want, got.Params); diff != "" {
		t.Errorf("params (-want +got):\n%s", diff)
	}
	if params["token"] != secret {
		t.Error("Record mutated the caller's params")
	}
}

func TestLongPollWakesOnRecord(t *testing.T) {
	t.Parallel()

	l := calllog.New(5)
	done := make(chan calllog.Result, 1)
	go func() { done <- l.Query(t.Context(), calllog.Query{Min: 1, Timeout: 10 * time.Second}) }()

	// Order does not matter: Query checks existing records before parking.
	l.Record(rec("a"))
	select {
	case res := <-done:
		if len(res.Calls) != 1 {
			t.Errorf("calls = %d, want 1", len(res.Calls))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long poll did not wake")
	}
}

func TestLongPollTimesOutAndHonorsContext(t *testing.T) {
	t.Parallel()

	l := calllog.New(5)
	if res := l.Query(t.Context(), calllog.Query{Min: 1, Timeout: 20 * time.Millisecond}); len(res.Calls) != 0 {
		t.Errorf("timeout poll returned %d calls, want 0", len(res.Calls))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if res := l.Query(ctx, calllog.Query{Min: 1, Timeout: 10 * time.Second}); len(res.Calls) != 0 {
		t.Errorf("cancelled poll returned %d calls, want 0", len(res.Calls))
	}
}

func TestClearKeepsSeqMonotonicAndBumpsGeneration(t *testing.T) {
	t.Parallel()

	l := calllog.New(5)
	l.Record(rec("a"))
	before := l.Query(t.Context(), calllog.Query{}).Generation
	l.Clear()
	l.Record(rec("b"))
	res := l.Query(t.Context(), calllog.Query{})
	if diff := cmp.Diff([]int64{2}, seqs(res)); diff != "" {
		t.Errorf("seq after clear (-want +got):\n%s", diff)
	}
	if res.Generation != before+1 {
		t.Errorf("generation = %d, want %d", res.Generation, before+1)
	}
}

func TestClearWakesParkedPoller(t *testing.T) {
	t.Parallel()

	l := calllog.New(5)
	done := make(chan calllog.Result, 1)
	go func() { done <- l.Query(t.Context(), calllog.Query{Min: 1, Timeout: 10 * time.Second}) }()
	// Wait until the poller is parked, then clear: it must wake (generation changed) without a record.
	deadline := time.Now().Add(5 * time.Second)
	for l.Waiters() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("poller never parked")
		}
		runtime.Gosched()
	}
	l.Clear()
	select {
	case res := <-done:
		if res.Generation != 1 {
			t.Errorf("generation = %d, want 1", res.Generation)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clear did not wake the poller")
	}
}
