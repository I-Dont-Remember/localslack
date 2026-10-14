package service_test

import (
	"errors"
	"testing"

	"localslack/internal/service"
)

func TestListConversationsByPrivacy(t *testing.T) {
	t.Parallel()
	owner := func(e *accessEnv) service.User { return e.owner }
	other := func(e *accessEnv) service.User { return e.otherHuman }
	tests := []struct {
		name  string
		types []string
		as    func(*accessEnv) service.User
		has   []string // names that must be listed
		not   []string // names that must not be
	}{
		{"public only default", nil, owner, []string{"pub", "general"}, []string{"priv"}},
		{"private for member", []string{"private_channel"}, owner, []string{"priv"}, []string{"pub", "general"}},
		{"private for non-member", []string{"private_channel"}, other, nil, []string{"priv", "pub"}},
		{"both", []string{"public_channel", "private_channel"}, owner, []string{"priv", "pub"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			convs, err := e.svc.ListAll(t.Context(), tt.as(e), tt.types, false)
			if err != nil {
				t.Fatalf("ListAll: %v", err)
			}
			got := map[string]bool{}
			for _, c := range convs {
				got[c.Name] = true
				if c.Name == "priv" && !c.IsPrivate {
					t.Errorf("priv not marked private")
				}
			}
			for _, n := range tt.has {
				if !got[n] {
					t.Errorf("missing %q in %v", n, got)
				}
			}
			for _, n := range tt.not {
				if got[n] {
					t.Errorf("unexpected %q", n)
				}
			}
		})
	}
}

func TestEnsureMember(t *testing.T) {
	t.Parallel()
	e := newAccessEnv(t, nil)
	ctx := t.Context()
	for _, ch := range []string{e.pub, e.pub, e.priv, e.archived} { // twice: idempotent; private/archived untouched
		if err := e.svc.EnsureMember(ctx, e.otherHuman, ch); err != nil {
			t.Fatalf("EnsureMember(%s): %v", ch, err)
		}
	}
	if _, err := e.svc.PostMessage(ctx, e.otherHuman, service.PostOpts{Channel: e.pub, Content: service.Content{Text: "hi"}}); err != nil {
		t.Errorf("post after join: %v", err)
	}
	if _, err := e.svc.Conversation(ctx, e.otherHuman, e.priv); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("private after EnsureMember err = %v, want channel_not_found", err)
	}
	if err := e.svc.EnsureMember(ctx, e.otherHuman, "CNOPE"); !errors.Is(err, service.ErrChannelNotFound) {
		t.Errorf("unknown channel err = %v", err)
	}
}
