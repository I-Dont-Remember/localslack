package web

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"localslack/internal/service"
)

type navItem struct {
	ID, Title string
	Active    bool
}

type msgView struct {
	ChannelID, TS, User, Time, Text string
	ReplyCount                      int
	RepliesLabel                    string
	InThread                        bool
	// Root is the thread root's ts for messages shown in a thread pane.
	Root      string
	Reactions []reactionView
	Picker    []emoji
}

type threadView struct {
	RootTS   string
	Messages []msgView
}

type pageData struct {
	Actor     service.User
	Users     []service.User
	Path      string
	Channels  []navItem
	DMs       []navItem
	ChannelID string
	Title     string
	Messages  []msgView
	Thread    *threadView
}

type switchItem struct{ Href, Label, UserID string }

func names(users []service.User) map[string]string {
	m := make(map[string]string, len(users))
	for _, u := range users {
		m[u.ID] = u.Name
	}
	return m
}

// msgViews maps messages for display; root is the thread root's ts inside a thread pane, "" otherwise.
func msgViews(msgs []service.Message, who map[string]string, root string) []msgView {
	out := make([]msgView, 0, len(msgs))
	picker := pickerEmoji()
	for _, m := range msgs {
		v := msgView{
			ChannelID: m.ChannelID, TS: m.TS, User: who[m.UserID], Time: clock(m.TS), Text: m.Text,
			ReplyCount: m.ReplyCount, InThread: root != "", Root: root,
			Reactions: reactionViews(m.Reactions), Picker: picker,
		}
		if v.User == "" {
			v.User = m.UserID
		}
		if m.ReplyCount == 1 {
			v.RepliesLabel = "1 reply"
		} else {
			v.RepliesLabel = strconv.Itoa(m.ReplyCount) + " replies"
		}
		out = append(out, v)
	}
	return out
}

func clock(ts string) string {
	sec, _, _ := strings.Cut(ts, ".")
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(n, 0).UTC().Format("15:04")
}

// title renders a conversation's display name from the actor's point of view.
func title(c service.Conversation, who map[string]string, actor service.User) string {
	switch c.Kind {
	case service.KindIM:
		if c.Peer == actor.ID {
			return who[c.Peer] + " (you)"
		}
		return who[c.Peer]
	case service.KindMPIM:
		if c.Name != "" {
			return c.Name
		}
		return "Group message"
	}
	return c.Name
}

// page builds the shared shell: users, sidebar, and the open conversation.
func (w *Web) page(ctx context.Context, actor service.User, path, channelID string) (*pageData, error) {
	conv, err := w.svc.Conversation(ctx, actor, channelID)
	if err != nil {
		return nil, err
	}
	users, err := w.svc.Users(ctx)
	if err != nil {
		return nil, err
	}
	who := names(users)
	chans, err := w.svc.ListAll(ctx, actor, []string{"public_channel"}, true)
	if err != nil {
		return nil, err
	}
	dms, err := w.svc.ListAll(ctx, actor, []string{"im", "mpim"}, true)
	if err != nil {
		return nil, err
	}
	d := &pageData{
		Actor: actor, Users: users, Path: path, ChannelID: channelID, Title: title(conv, who, actor),
		Channels: nav(chans, channelID, func(c service.Conversation) string { return c.Name }),
		DMs:      nav(dms, channelID, func(c service.Conversation) string { return title(c, who, actor) }),
	}
	if conv.Kind == service.KindChannel {
		d.Title = "#" + conv.Name
	}
	d.Messages, err = w.channelMessagesView(ctx, actor, channelID, who)
	return d, err
}

func nav(cs []service.Conversation, active string, label func(service.Conversation) string) []navItem {
	out := make([]navItem, 0, len(cs))
	for _, c := range cs {
		out = append(out, navItem{ID: c.ID, Title: label(c), Active: c.ID == active})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

func (w *Web) channelMessagesView(ctx context.Context, actor service.User, channelID string, who map[string]string) ([]msgView, error) {
	page, err := w.svc.History(ctx, actor, channelID, service.PageOpts{Limit: historyLimit})
	if err != nil {
		return nil, err
	}
	msgs := page.Items
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 { // oldest first
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgViews(msgs, who, ""), nil
}

func (w *Web) threadMessagesView(ctx context.Context, actor service.User, channelID, ts string, who map[string]string) (*threadView, error) {
	page, err := w.svc.Replies(ctx, actor, channelID, ts, service.PageOpts{Limit: historyLimit})
	if err != nil {
		return nil, err
	}
	root := ts
	if len(page.Items) > 0 {
		root = page.Items[0].TS
	}
	return &threadView{RootTS: root, Messages: msgViews(page.Items, who, root)}, nil
}

func (w *Web) whoIs(ctx context.Context) (map[string]string, error) {
	users, err := w.svc.Users(ctx)
	if err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	return names(users), nil
}
