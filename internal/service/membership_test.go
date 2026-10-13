package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

// seededGeneral finds #general (everyone is a member after seeding).
func seededGeneral(e *accessEnv) string {
	all, err := e.svc.ListAll(context.Background(), e.owner, []string{"public_channel"}, false)
	if err != nil {
		return ""
	}
	for _, c := range all {
		if c.Name == "general" {
			return c.ID
		}
	}
	return ""
}

// memberIDs lists a channel's members as seen by viewer.
func memberIDs(t *testing.T, e *accessEnv, viewer service.User, ch string) []string {
	t.Helper()
	page, err := e.svc.Members(t.Context(), viewer, ch, service.PageOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("Members(%s): %v", ch, err)
	}
	return page.Items
}

func TestJoinChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		as          func(*accessEnv) service.User
		ch          func(*accessEnv) string
		wantErr     error
		wantAlready bool
	}{
		{"bot joins public", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.pub }, nil, false},
		{"human joins public", func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.pub }, nil, false},
		{"already a member", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, nil, true},
		{"private is hidden", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.priv }, service.ErrChannelNotFound, false},
		{"dm is hidden", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.dm }, service.ErrChannelNotFound, false},
		{"archived", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.archived }, service.ErrIsArchived, false},
		{"unknown", func(e *accessEnv) service.User { return e.bot }, func(*accessEnv) string { return "CNOPE" }, service.ErrChannelNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			as, ch := tt.as(e), tt.ch(e)

			conv, already, err := e.svc.JoinChannel(t.Context(), as, ch)

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if already != tt.wantAlready {
				t.Errorf("already = %v, want %v", already, tt.wantAlready)
			}
			if conv.ID != ch || !conv.IsMember {
				t.Errorf("conversation = {%s member %v}, want %s with is_member", conv.ID, conv.IsMember, ch)
			}
			if !slices.Contains(memberIDs(t, e, as, ch), as.ID) {
				t.Errorf("%s not in members after join", as.ID)
			}
		})
	}
}

func TestLeaveChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		as      func(*accessEnv) service.User
		ch      func(*accessEnv) string
		wantErr error
	}{
		{"member leaves public", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, nil},
		{"member leaves private", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.priv }, nil},
		{"non-member of public", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.pub }, service.ErrNotInChannel},
		{"non-member of private", func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.priv }, service.ErrChannelNotFound},
		{"general", func(e *accessEnv) service.User { return e.owner }, seededGeneral, service.ErrCantLeaveGeneral},
		{"archived", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.archived }, service.ErrIsArchived},
		{"dm", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.dm }, service.ErrUnsupportedChannelType},
		{"unknown", func(e *accessEnv) service.User { return e.owner }, func(*accessEnv) string { return "CNOPE" }, service.ErrChannelNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			as, ch := tt.as(e), tt.ch(e)

			err := e.svc.LeaveChannel(t.Context(), as, ch)

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				conv, err := e.svc.Conversation(t.Context(), as, ch) // hidden (private) or is_member false (public)
				if err == nil && conv.IsMember {
					t.Errorf("%s still a member after leave", as.ID)
				}
				if err != nil && !errors.Is(err, service.ErrChannelNotFound) {
					t.Errorf("Conversation after leave: %v", err)
				}
			}
		})
	}
}

func TestInviteToChannel(t *testing.T) {
	t.Parallel()

	type env = accessEnv
	one := func(f func(*env) string) func(*env) []string { return func(e *env) []string { return []string{f(e)} } }
	botID := func(e *env) string { return e.bot.ID }
	tests := []struct {
		name         string
		as           func(*env) service.User
		ch           func(*env) string
		users        func(*env) []string
		force        bool
		wantErr      error
		wantFailures []service.InviteFailure // by user placeholder resolved below
		wantAdded    func(*env) []string
	}{
		{"member invites bot to private", func(e *env) service.User { return e.owner }, func(e *env) string { return e.priv }, one(botID), false, nil, nil, func(e *env) []string { return []string{e.bot.ID} }},
		{
			"several users to public", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(e *env) []string { return []string{e.bot.ID, e.otherHuman.ID} }, false, nil, nil,
			func(e *env) []string { return []string{e.bot.ID, e.otherHuman.ID} },
		},
		{"no users", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub }, func(*env) []string { return nil }, false, service.ErrNoUsers, nil, nil},
		{"non-member inviter, public", func(e *env) service.User { return e.otherHuman }, func(e *env) string { return e.pub }, one(botID), false, service.ErrNotInChannel, nil, nil},
		{"non-member inviter, private", func(e *env) service.User { return e.otherHuman }, func(e *env) string { return e.priv }, one(botID), false, service.ErrChannelNotFound, nil, nil},
		{"archived", func(e *env) service.User { return e.owner }, func(e *env) string { return e.archived }, one(botID), false, service.ErrIsArchived, nil, nil},
		{"dm", func(e *env) service.User { return e.owner }, func(e *env) string { return e.dm }, one(botID), false, service.ErrUnsupportedChannelType, nil, nil},
		{"unknown channel", func(e *env) service.User { return e.owner }, func(*env) string { return "CNOPE" }, one(botID), false, service.ErrChannelNotFound, nil, nil},
		{
			"unknown user", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(*env) []string { return []string{"UNOPE"} }, false, service.ErrUserNotFound,
			[]service.InviteFailure{{User: "UNOPE", Err: service.ErrUserNotFound}},
			nil,
		},
		{
			"self", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(e *env) []string { return []string{e.owner.ID} }, false, service.ErrCantInviteSelf, nil, nil,
		},
		{
			"one bad user rejects the batch", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(e *env) []string { return []string{e.bot.ID, "UNOPE"} }, false, service.ErrUserNotFound,
			[]service.InviteFailure{{User: "UNOPE", Err: service.ErrUserNotFound}},
			func(*env) []string { return nil },
		},
		{
			"force keeps the valid users", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(e *env) []string { return []string{e.bot.ID, "UNOPE"} }, true, nil, nil, func(e *env) []string { return []string{e.bot.ID} },
		},
		{
			"force with only bad users still fails", func(e *env) service.User { return e.owner }, func(e *env) string { return e.pub },
			func(*env) []string { return []string{"UNOPE"} }, true, service.ErrUserNotFound,
			[]service.InviteFailure{{User: "UNOPE", Err: service.ErrUserNotFound}},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			as, ch := tt.as(e), tt.ch(e)
			before := memberIDs(t, e, e.owner, e.pub)

			conv, err := e.svc.InviteToChannel(t.Context(), as, ch, tt.users(e), tt.force)

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			var ie *service.InviteError
			switch {
			case tt.wantFailures != nil:
				if !errors.As(err, &ie) {
					t.Fatalf("err = %T, want *InviteError", err)
				}
				if diff := cmp.Diff(tt.wantFailures, ie.Failures, cmp.Comparer(errors.Is)); diff != "" {
					t.Errorf("failures (-want +got):\n%s", diff)
				}
			case err == nil && conv.ID != ch:
				t.Errorf("conversation id = %q, want %q", conv.ID, ch)
			}
			if tt.wantAdded != nil && ch == e.pub {
				want := append(slices.Clone(before), tt.wantAdded(e)...)
				slices.Sort(want)
				if diff := cmp.Diff(want, memberIDs(t, e, e.owner, e.pub)); diff != "" {
					t.Errorf("members (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestInviteReportsAlreadyInChannel(t *testing.T) {
	t.Parallel()
	e := newAccessEnv(t, nil)
	if _, err := e.svc.InviteToChannel(t.Context(), e.owner, e.pub, []string{e.bot.ID}, false); err != nil {
		t.Fatalf("first invite: %v", err)
	}

	_, err := e.svc.InviteToChannel(t.Context(), e.owner, e.pub, []string{e.bot.ID}, false)

	var ie *service.InviteError
	if !errors.As(err, &ie) || !errors.Is(err, service.ErrAlreadyInChannel) {
		t.Fatalf("err = %v, want InviteError wrapping ErrAlreadyInChannel", err)
	}
}

func TestKickFromChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		as      func(*accessEnv) service.User
		ch      func(*accessEnv) string
		user    func(*accessEnv) string
		wantErr error
	}{
		{"kick a member", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.bot.ID }, nil},
		{"user not in channel", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.otherHuman.ID }, service.ErrNotInChannel},
		{"unknown user", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, func(*accessEnv) string { return "UNOPE" }, service.ErrUserNotFound},
		{"self", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.owner.ID }, service.ErrCantKickSelf},
		{"general", func(e *accessEnv) service.User { return e.owner }, seededGeneral, func(e *accessEnv) string { return e.bot.ID }, service.ErrCantKickFromGeneral},
		{"non-member kicker, private", func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.priv }, func(e *accessEnv) string { return e.owner.ID }, service.ErrChannelNotFound},
		{"non-member kicker, public", func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.owner.ID }, service.ErrNotInChannel},
		{"archived", func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.archived }, func(e *accessEnv) string { return e.bot.ID }, service.ErrIsArchived},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			if _, err := e.svc.InviteToChannel(t.Context(), e.owner, e.pub, []string{e.bot.ID}, false); err != nil {
				t.Fatalf("setup invite: %v", err)
			}

			err := e.svc.KickFromChannel(t.Context(), tt.as(e), tt.ch(e), tt.user(e))

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && slices.Contains(memberIDs(t, e, e.owner, e.pub), e.bot.ID) {
				t.Error("kicked user is still a member")
			}
		})
	}
}

// Membership changes feed the access rules: a joined bot can read, a left one cannot.
func TestMembershipDrivesAccess(t *testing.T) {
	t.Parallel()
	e := newAccessEnv(t, nil)
	hist := func() error {
		_, err := e.svc.History(t.Context(), e.bot, e.pub, service.PageOpts{})
		return err
	}
	if err := hist(); !errors.Is(err, service.ErrNotInChannel) {
		t.Fatalf("before join: err = %v, want not_in_channel", err)
	}
	if _, _, err := e.svc.JoinChannel(t.Context(), e.bot, e.pub); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := hist(); err != nil {
		t.Errorf("after join: err = %v, want nil", err)
	}
	if err := e.svc.LeaveChannel(t.Context(), e.bot, e.pub); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := hist(); !errors.Is(err, service.ErrNotInChannel) {
		t.Errorf("after leave: err = %v, want not_in_channel", err)
	}
}

func TestSetMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ch      func(*accessEnv) string
		user    func(*accessEnv) string
		member  bool
		wantErr error
	}{
		{"add bot to public", func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.bot.ID }, true, nil},
		{"add bot to private", func(e *accessEnv) string { return e.priv }, func(e *accessEnv) string { return e.bot.ID }, true, nil},
		{"add human to dm", func(e *accessEnv) string { return e.dm }, func(e *accessEnv) string { return e.bot.ID }, true, nil},
		{"add to archived", func(e *accessEnv) string { return e.archived }, func(e *accessEnv) string { return e.bot.ID }, true, nil},
		{"add twice (idempotent)", func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.owner.ID }, true, nil},
		{"remove owner", func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.owner.ID }, false, nil},
		{"remove non-member (idempotent)", func(e *accessEnv) string { return e.pub }, func(e *accessEnv) string { return e.bot.ID }, false, nil},
		{"unknown channel", func(*accessEnv) string { return "CNOPE" }, func(e *accessEnv) string { return e.bot.ID }, true, service.ErrChannelNotFound},
		{"unknown user", func(e *accessEnv) string { return e.pub }, func(*accessEnv) string { return "UNOPE" }, true, service.ErrUserNotFound},
		{"unknown user on remove", func(e *accessEnv) string { return e.pub }, func(*accessEnv) string { return "UNOPE" }, false, service.ErrUserNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			ch, user := tt.ch(e), tt.user(e)

			err := e.svc.SetMembership(t.Context(), ch, user, tt.member)

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// the owner can see every channel in the env (member, or public)
			page, err := e.svc.Members(t.Context(), e.owner, ch, service.PageOpts{Limit: 1000})
			if err != nil {
				t.Fatalf("Members: %v", err)
			}
			got := slices.Contains(page.Items, user)
			if got != tt.member {
				t.Errorf("member = %v after SetMembership(%v), want %v", got, tt.member, tt.member)
			}
		})
	}
}
