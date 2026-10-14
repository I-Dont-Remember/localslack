package events_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
	"localslack/internal/service"
)

var _ events.Dispatcher = events.Noop{}

func TestNoopDispatcherLifecycle(t *testing.T) {
	t.Parallel()

	var d events.Dispatcher = events.Noop{}
	d.Publish(service.MessagePosted{})
	d.Pause()
	d.Reset()
	d.Resume()
	if err := d.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

func TestDeliveryJSONMatchesContract(t *testing.T) {
	t.Parallel()

	d := events.Delivery{
		ID: "d1", Seq: 1, EventID: "Ev1", EventType: "message", AppID: "A1", Transport: "http",
		State: events.StateRetrying, Payload: json.RawMessage(`{"type":"event_callback"}`),
		Attempts: []events.Attempt{{N: 1, At: time.Unix(0, 0).UTC(), Status: 500, SignatureMode: "valid"}},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"id", "seq", "event_id", "event_type", "app_id", "transport", "state", "injected", "payload", "attempts"} {
		if _, ok := got[k]; !ok {
			t.Errorf("delivery JSON lacks %q", k)
		}
	}
	attempt := got["attempts"].([]any)[0].(map[string]any)
	var keys []string
	for k := range attempt {
		keys = append(keys, k)
	}
	want := []string{"n", "at", "status", "latency_ms", "error", "retry_reason", "signature_mode", "timestamp", "signature", "response_body"}
	if diff := cmp.Diff(want, keys, cmpSortStrings()); diff != "" {
		t.Errorf("attempt keys (-want +got):\n%s", diff)
	}
}

func cmpSortStrings() cmp.Option {
	return cmp.Transformer("sort", func(in []string) []string {
		out := append([]string(nil), in...)
		slices.Sort(out)
		return out
	})
}
