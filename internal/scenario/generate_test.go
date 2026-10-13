package scenario_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/scenario"
)

func genNow() time.Time { return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC) }

func TestGenerateIsDeterministicAndValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		g    scenario.Generate
	}{
		{"defaults", scenario.Generate{Seed: "a"}},
		{"small", scenario.Generate{Seed: "b", Users: 3, Channels: 2, MessagesPerChannel: 5, ThreadRatio: 0.5}},
		{"private", scenario.Generate{Seed: "c", Users: 12, Channels: 6, MessagesPerChannel: 9, ThreadRatio: 0.3, PrivateChannels: 3}},
		{"tiny", scenario.Generate{Seed: "d", Users: 1, Channels: 1, MessagesPerChannel: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, err := scenario.Expand(tt.g, genNow())
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			b, err := scenario.Expand(tt.g, genNow())
			if err != nil {
				t.Fatalf("Generate again: %v", err)
			}
			if diff := cmp.Diff(a, b); diff != "" {
				t.Errorf("same input, different output (-a +b):\n%s", diff)
			}
			if issues := scenario.Validate(a, scenario.Reserved{}); len(issues) != 0 {
				t.Errorf("generated fixture invalid: %+v", issues)
			}
			if a.Generate != nil {
				t.Errorf("Generate field must be expanded away, got %+v", a.Generate)
			}
		})
	}
}

func TestGenerateShape(t *testing.T) {
	t.Parallel()

	g := scenario.Generate{Seed: "shape", Users: 20, Channels: 8, MessagesPerChannel: 30, ThreadRatio: 0.3, PrivateChannels: 2}
	fx, err := scenario.Expand(g, genNow())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var humans, bots int
	for _, u := range fx.Users {
		if u.IsBot {
			bots++
		} else {
			humans++
		}
	}
	if humans != 20 || bots != 1 {
		t.Errorf("humans=%d bots=%d, want 20 and 1", humans, bots)
	}
	if len(fx.Channels) != 8 || fx.Channels[0].Name != "general" || fx.Channels[0].IsPrivate {
		t.Errorf("channels = %d first=%+v, want 8 starting with public general", len(fx.Channels), fx.Channels[0])
	}
	private := 0
	for _, c := range fx.Channels {
		if c.IsPrivate {
			private++
			if len(c.Members) < 2 || len(c.Members) >= len(fx.Users) {
				t.Errorf("private %s has %d members, want a proper subset of %d", c.Name, len(c.Members), len(fx.Users))
			}
		} else if len(c.Members) != len(fx.Users) {
			t.Errorf("public %s has %d members, want everyone (%d)", c.Name, len(c.Members), len(fx.Users))
		}
	}
	if private != 2 {
		t.Errorf("private channels = %d, want 2", private)
	}
	if want := 8 * 30; len(fx.Messages) != want {
		t.Errorf("messages = %d, want %d", len(fx.Messages), want)
	}
	replies := 0
	for _, m := range fx.Messages {
		if m.ThreadTS != "" {
			replies++
		}
		if m.TS > fmt.Sprintf("%d.999999", genNow().Unix()) {
			t.Errorf("ts %s is in the future", m.TS)
		}
	}
	if ratio := float64(replies) / float64(len(fx.Messages)); ratio < 0.15 || ratio > 0.45 {
		t.Errorf("reply ratio = %.2f, want about 0.3", ratio)
	}
}

func TestGenerateSeedsDiffer(t *testing.T) {
	t.Parallel()

	a, _ := scenario.Expand(scenario.Generate{Seed: "x"}, genNow())
	b, _ := scenario.Expand(scenario.Generate{Seed: "y"}, genNow())
	if cmp.Equal(a, b) {
		t.Error("different seeds produced identical fixtures")
	}
}

func TestGenerateScaleValidates(t *testing.T) {
	t.Parallel()

	fx, err := scenario.Expand(scenario.Generate{Seed: "scale", Users: 400, Channels: 50, MessagesPerChannel: 40, ThreadRatio: 0.2, PrivateChannels: 5}, genNow())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if issues := scenario.Validate(fx, scenario.Reserved{}); len(issues) != 0 {
		t.Fatalf("invalid: %+v", issues[:min(5, len(issues))])
	}
	if len(fx.Messages) != 2000 || len(fx.Users) != 401 {
		t.Errorf("messages=%d users=%d, want 2000 and 401", len(fx.Messages), len(fx.Users))
	}
}

func TestGenerateRejectsBadParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		g    scenario.Generate
		want []scenario.Issue
	}{
		{"users too many", scenario.Generate{Users: 5000}, []scenario.Issue{is("/generate/users", "users must be 1..2000")}},
		{"negative users", scenario.Generate{Users: -1}, []scenario.Issue{is("/generate/users", "users must be 1..2000")}},
		{"negative messages", scenario.Generate{MessagesPerChannel: -1}, []scenario.Issue{is("/generate/messages_per_channel", "messages_per_channel must be 1..5000")}},
		{"channels too many", scenario.Generate{Channels: 1001}, []scenario.Issue{is("/generate/channels", "channels must be 1..1000")}},
		{"messages too many", scenario.Generate{MessagesPerChannel: 5001}, []scenario.Issue{is("/generate/messages_per_channel", "messages_per_channel must be 1..5000")}},
		{"ratio", scenario.Generate{ThreadRatio: 1.5}, []scenario.Issue{is("/generate/thread_ratio", "thread_ratio must be between 0 and 1")}},
		{"private > channels", scenario.Generate{Channels: 3, PrivateChannels: 3}, []scenario.Issue{is("/generate/private_channels", "private_channels must be less than channels (general is public)")}},
		{"total cap", scenario.Generate{Channels: 1000, MessagesPerChannel: 1000}, []scenario.Issue{is("/generate", "channels x messages_per_channel must be at most 200000")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, tt.g.Issues("/generate")); diff != "" {
				t.Errorf("issues (-want +got):\n%s", diff)
			}
			if _, err := scenario.Expand(tt.g, genNow()); err == nil {
				t.Error("Generate = nil error, want validation error")
			}
		})
	}
}
