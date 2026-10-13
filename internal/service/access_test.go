package service_test

import (
	"context"
	"errors"
	"testing"

	"localslack/internal/service"
)

type accessEnv struct {
	svc                    *service.Service
	owner, otherHuman, bot service.User
	pub, priv, archived    string // owner is the only member
	dm                     string // owner + otherHuman
}

// newAccessEnv builds channels the bot and otherHuman are NOT in. A non-nil
// botScopes registers an app for the bot with exactly those scopes.
func newAccessEnv(t *testing.T, botScopes []string) *accessEnv {
	t.Helper()
	svc, _ := newSvc(t)
	ctx := t.Context()
	us, err := svc.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	e := &accessEnv{svc: svc}
	for _, u := range us {
		switch {
		case u.IsBot && e.bot.ID == "":
			e.bot = u
		case !u.IsBot && e.owner.ID == "":
			e.owner = u
		case !u.IsBot && e.otherHuman.ID == "":
			e.otherHuman = u
		}
	}
	if botScopes != nil {
		svc.SetApps(service.App{ID: "A9", Name: "scoped", BotToken: "xoxb-scoped", BotUserID: e.bot.ID, Scopes: botScopes})
	}
	mk := func(name string, opts ...service.CreateOpt) string {
		c, err := svc.CreateChannel(ctx, e.owner, name, opts...)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return c.ID
	}
	e.pub, e.priv, e.archived = mk("pub"), mk("priv", service.CreatePrivate()), mk("arch")
	if err := svc.ArchiveChannel(ctx, e.owner, e.archived); err != nil {
		t.Fatalf("archive: %v", err)
	}
	dm, err := svc.OpenDM(ctx, e.owner, []string{e.otherHuman.ID})
	if err != nil {
		t.Fatalf("OpenDM: %v", err)
	}
	e.dm = dm.ID
	return e
}

type accessOp struct {
	name string
	run  func(ctx context.Context, e *accessEnv, as service.User, ch string) error
}

func accessOps() []accessOp {
	return []accessOp{
		{"history", func(ctx context.Context, e *accessEnv, as service.User, ch string) error {
			_, err := e.svc.History(ctx, as, ch, service.PageOpts{})
			return err
		}},
		{"info", func(ctx context.Context, e *accessEnv, as service.User, ch string) error {
			_, err := e.svc.Conversation(ctx, as, ch)
			return err
		}},
		{"members", func(ctx context.Context, e *accessEnv, as service.User, ch string) error {
			_, err := e.svc.Members(ctx, as, ch, service.PageOpts{})
			return err
		}},
		{"post", func(ctx context.Context, e *accessEnv, as service.User, ch string) error {
			_, err := e.svc.PostMessage(ctx, as, service.PostOpts{Channel: ch, Content: service.Content{Text: "x"}})
			return err
		}},
		{"ephemeral", func(ctx context.Context, e *accessEnv, as service.User, ch string) error {
			_, err := e.svc.PostEphemeral(ctx, as, service.EphemeralOpts{Channel: ch, User: e.owner.ID, Content: service.Content{Text: "x"}})
			return err
		}},
	}
}

func TestAccessMatrix(t *testing.T) {
	t.Parallel()
	nf, nic, arch := service.ErrChannelNotFound, service.ErrNotInChannel, service.ErrIsArchived
	allPrivate := map[string]error{"history": nf, "info": nf, "members": nf, "post": nf, "ephemeral": nf}
	tests := []struct {
		name      string
		botScopes []string
		as        func(*accessEnv) service.User
		ch        func(*accessEnv) string
		want      map[string]error // by op; missing = allowed
	}{
		{"owner in public", nil, func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.pub }, nil},
		{"owner in private", nil, func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.priv }, nil},
		{
			"human non-member: reads public, cannot post", nil,
			func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.pub },
			map[string]error{"post": nic, "ephemeral": nic},
		},
		{
			"default bot non-member of public: write.public posts, cannot read", nil,
			func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.pub },
			map[string]error{"history": nic},
		},
		{
			"chat:write-only bot non-member of public",
			[]string{"chat:write"},
			func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.pub },
			map[string]error{"history": nic, "post": nic, "ephemeral": nic},
		},
		{
			"chat:write.public bot non-member of public",
			[]string{"chat:write", "chat:write.public"},
			func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.pub },
			map[string]error{"history": nic},
		},
		{"human non-member of private", nil, func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.priv }, allPrivate},
		{"bot non-member of private", nil, func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.priv }, allPrivate},
		{"bot non-member of dm", nil, func(e *accessEnv) service.User { return e.bot }, func(e *accessEnv) string { return e.dm }, allPrivate},
		{"dm member", nil, func(e *accessEnv) service.User { return e.otherHuman }, func(e *accessEnv) string { return e.dm }, nil},
		{
			"archived", nil, func(e *accessEnv) service.User { return e.owner }, func(e *accessEnv) string { return e.archived },
			map[string]error{"post": arch, "ephemeral": arch},
		},
	}
	for _, tt := range tests {
		for _, op := range accessOps() {
			t.Run(tt.name+"/"+op.name, func(t *testing.T) {
				t.Parallel()
				e := newAccessEnv(t, tt.botScopes)
				got := op.run(t.Context(), e, tt.as(e), tt.ch(e))
				if want := tt.want[op.name]; !errors.Is(got, want) {
					t.Errorf("err = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestBotPostWithWritePublicDoesNotJoin(t *testing.T) {
	t.Parallel()
	e := newAccessEnv(t, nil)
	if _, err := e.svc.PostMessage(t.Context(), e.bot, service.PostOpts{Channel: e.pub, Content: service.Content{Text: "hi"}}); err != nil {
		t.Fatalf("post: %v", err)
	}
	c, err := e.svc.Conversation(t.Context(), e.bot, e.pub)
	if err != nil || c.IsMember {
		t.Errorf("after post: %+v, %v; want bot still not a member", c, err)
	}
}

func TestWriteAndReactAccess(t *testing.T) {
	t.Parallel()
	nf, nic, arch := service.ErrChannelNotFound, service.ErrNotInChannel, service.ErrIsArchived
	tests := []struct {
		name, op string
		want     error
	}{
		{"update public non-member's own message ok via write.public", "update-own", nil},
		{"react non-member public", "react-nonmember", nic},
		{"react non-member private", "react-private", nf},
		{"react archived", "react-archived", arch},
		{"update archived", "update-archived", arch},
		{"delete private non-member", "delete-private", nf},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccessEnv(t, nil)
			ctx := t.Context()
			post := func(as service.User, ch string) string {
				m, err := e.svc.PostMessage(ctx, as, service.PostOpts{Channel: ch, Content: service.Content{Text: "m"}})
				if err != nil {
					t.Fatalf("post: %v", err)
				}
				return m.TS
			}
			archivedWithMessage := func() (string, string) {
				c, err := e.svc.CreateChannel(ctx, e.owner, "arch2")
				if err != nil {
					t.Fatalf("create: %v", err)
				}
				ts := post(e.owner, c.ID)
				if err := e.svc.ArchiveChannel(ctx, e.owner, c.ID); err != nil {
					t.Fatalf("archive: %v", err)
				}
				return c.ID, ts
			}
			var got error
			switch tt.op {
			case "update-own":
				ts := post(e.bot, e.pub)
				_, got = e.svc.UpdateMessage(ctx, e.bot, service.UpdateOpts{Channel: e.pub, TS: ts, Content: service.Content{Text: "n"}})
			case "react-nonmember":
				got = e.svc.AddReaction(ctx, e.otherHuman, e.pub, post(e.owner, e.pub), "tada")
			case "react-private":
				got = e.svc.AddReaction(ctx, e.otherHuman, e.priv, post(e.owner, e.priv), "tada")
			case "react-archived":
				ch, ts := archivedWithMessage()
				got = e.svc.AddReaction(ctx, e.owner, ch, ts, "tada")
			case "update-archived":
				ch, ts := archivedWithMessage()
				_, got = e.svc.UpdateMessage(ctx, e.owner, service.UpdateOpts{Channel: ch, TS: ts, Content: service.Content{Text: "n"}})
			case "delete-private":
				got = e.svc.DeleteMessage(ctx, e.otherHuman, e.priv, post(e.owner, e.priv))
			}
			if !errors.Is(got, tt.want) {
				t.Errorf("err = %v, want %v", got, tt.want)
			}
		})
	}
}
