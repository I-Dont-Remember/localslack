package admin

import (
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParsePoll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		want    poll
		wantErr bool
	}{
		{"empty", "", poll{}, false},
		{"all set", "since=7&limit=5&min=2&timeout_ms=1500", poll{Since: 7, Limit: 5, Min: 2, Timeout: 1500 * time.Millisecond}, false},
		{"min alone waits the default", "min=1", poll{Min: 1, Timeout: defaultPollTimeout}, false},
		{"timeout is capped", "min=1&timeout_ms=999999", poll{Min: 1, Timeout: maxPollTimeout}, false},
		{"timeout without min is kept", "timeout_ms=40", poll{Timeout: 40 * time.Millisecond}, false},
		{"negative since", "since=-1", poll{}, true},
		{"not a number", "limit=lots", poll{}, true},
		{"every problem is reported", "since=x&min=y", poll{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}

			got, err := parsePoll(v)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("poll (-want +got):\n%s", diff)
			}
		})
	}
}
