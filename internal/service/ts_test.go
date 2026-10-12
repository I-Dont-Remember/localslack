package service

import (
	"testing"
	"time"
)

func TestTSGenAdvanceToIsForwardOnly(t *testing.T) {
	t.Parallel()

	past := time.Unix(100, 0)
	tests := []struct {
		name    string
		start   string
		advance string
		want    string // next ts with a clock in the past
	}{
		{"moves forward", "", "200.000005", "200.000006"},
		{"never moves back", "300.000001", "200.000000", "300.000002"},
		{"equal is a no-op", "300.000001", "300.000001", "300.000002"},
		{"empty means no messages", "300.000001", "", "300.000002"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, err := newTSGen(tt.start)
			if err != nil {
				t.Fatalf("newTSGen: %v", err)
			}

			if err := g.advanceTo(tt.advance); err != nil {
				t.Fatalf("advanceTo: %v", err)
			}

			if got := g.next(past); got != tt.want {
				t.Errorf("next = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTSGenAdvanceToRejectsMalformed(t *testing.T) {
	t.Parallel()

	g, err := newTSGen("")
	if err != nil {
		t.Fatalf("newTSGen: %v", err)
	}
	if err := g.advanceTo("garbage"); err == nil {
		t.Error("advanceTo(garbage) = nil, want error")
	}
}
