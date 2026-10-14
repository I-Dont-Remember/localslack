package events_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		policy      events.URLPolicy
		manifestURL string
		deliverTo   string
		want        events.Target
	}{
		{
			name:        "loopback manifest url is used as is",
			manifestURL: "http://127.0.0.1:3000/slack/events",
			want:        events.Target{URL: "http://127.0.0.1:3000/slack/events"},
		},
		{
			name:        "private manifest url is used",
			manifestURL: "http://192.168.1.5:3000/e",
			want:        events.Target{URL: "http://192.168.1.5:3000/e"},
		},
		{
			name:        "localhost manifest url is used",
			manifestURL: "http://localhost:3000/e",
			want:        events.Target{URL: "http://localhost:3000/e"},
		},
		{
			name:        "remote manifest url is blocked",
			manifestURL: "https://example.com/slack/events",
			want: events.Target{
				Blocked: true,
				Reason:  `manifest request_url host "example.com" is not loopback/private; set deliver_to or allow remote request URLs`,
			},
		},
		{
			name:        "remote manifest url allowed by flag",
			policy:      events.URLPolicy{AllowRemote: true},
			manifestURL: "https://example.com/slack/events",
			want:        events.Target{URL: "https://example.com/slack/events"},
		},
		{
			name:        "host-only deliver_to keeps the manifest path",
			manifestURL: "https://example.com/hooks/slack?x=1",
			deliverTo:   "http://localhost:4000",
			want:        events.Target{URL: "http://localhost:4000/hooks/slack?x=1"},
		},
		{
			name:        "deliver_to with a path is verbatim",
			manifestURL: "https://example.com/hooks/slack",
			deliverTo:   "http://localhost:4000/custom",
			want:        events.Target{URL: "http://localhost:4000/custom"},
		},
		{
			name:      "host-only deliver_to without manifest url uses the Bolt default path",
			deliverTo: "http://localhost:4000/",
			want:      events.Target{URL: "http://localhost:4000/slack/events"},
		},
		{
			name:        "per-app deliver_to beats global",
			policy:      events.URLPolicy{DeliverTo: "http://localhost:5000"},
			manifestURL: "http://localhost:1/e",
			deliverTo:   "http://localhost:4000",
			want:        events.Target{URL: "http://localhost:4000/e"},
		},
		{
			name:        "global deliver_to beats manifest",
			policy:      events.URLPolicy{DeliverTo: "http://localhost:5000"},
			manifestURL: "https://example.com/e",
			want:        events.Target{URL: "http://localhost:5000/e"},
		},
		{
			name:        "remote deliver_to blocked without flag",
			manifestURL: "http://localhost:1/e",
			deliverTo:   "https://abc.example.net",
			want: events.Target{
				Blocked: true,
				Reason:  `deliver_to host "abc.example.net" is not loopback/private; allow remote request URLs to use it`,
			},
		},
		{
			name: "nothing configured",
			want: events.Target{Blocked: true, Reason: "no request_url in the manifest and no deliver_to"},
		},
		{
			name:      "garbage deliver_to",
			deliverTo: "not a url",
			want:      events.Target{Blocked: true, Reason: `deliver_to "not a url" is not an http(s) URL`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.policy.Resolve(tt.manifestURL, tt.deliverTo)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Resolve (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidDeliverTo(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]bool{
		"http://localhost:3000": true, "https://x.test/p": true, "": false, "ftp://x": false, "localhost:3000": false,
	} {
		if got := events.ValidDeliverTo(in); got != want {
			t.Errorf("ValidDeliverTo(%q) = %v, want %v", in, got, want)
		}
	}
}
