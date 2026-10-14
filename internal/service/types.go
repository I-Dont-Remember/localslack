package service

import (
	"context"
	"encoding/json"
	"slices"

	"localslack/internal/store"
)

// Conversation is a channel, DM or group DM as seen by one caller.
type Conversation struct {
	ID, TeamID, Name, Kind, Creator string
	Created                         int64
	IsArchived                      bool
	IsPrivate                       bool
	// Peer is the other user of an im (the caller itself for a self-DM).
	Peer       string
	IsMember   bool
	NumMembers int
}

// Edit marks a message as edited by User at TS.
type Edit struct{ User, TS string }

// Reaction is one emoji on a message and the users who added it.
type Reaction struct {
	Name  string
	Users []string
	Count int
}

// Message is a posted message. ThreadTS is Slack-shaped: the root's ts for a
// root with replies and for every reply, empty otherwise.
type Message struct {
	ChannelID, TS, UserID, Text string
	ThreadTS                    string
	ReplyCount                  int
	LatestReply                 string
	// ParentUserID is the thread root's author, set on replies by Replies.
	ParentUserID string

	// BotID is set when a bot user authored the message.
	BotID, Subtype string
	Blocks         json.RawMessage
	Attachments    json.RawMessage
	Metadata       json.RawMessage
	Edited         *Edit
	Reactions      []Reaction
}

func msgFrom(m store.Message) Message {
	thread := m.ThreadTS
	if thread == "" && m.ReplyCount > 0 {
		thread = m.TS
	}
	out := Message{
		ChannelID: m.ChannelID, TS: m.TS, UserID: m.UserID, Text: m.Text,
		ThreadTS: thread, ReplyCount: m.ReplyCount, LatestReply: m.LatestReply,
		BotID: m.BotID, Subtype: m.Subtype, Blocks: m.Blocks, Attachments: m.Attachments, Metadata: m.Metadata,
	}
	if m.Edited != nil {
		out.Edited = &Edit{User: m.Edited.User, TS: m.Edited.TS}
	}
	return out
}

// enrich adds membership info for caller to store conversations.
func (s *Service) enrich(ctx context.Context, caller User, cs []store.Conversation) ([]Conversation, error) {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	members, err := s.st.MembersOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(cs))
	for _, c := range cs {
		m := members[c.ID]
		conv := convFrom(c, m)
		conv.IsMember = slices.Contains(m, caller.ID)
		if c.Kind == store.KindIM {
			conv.Peer = peerOf(m, caller.ID)
		}
		out = append(out, conv)
	}
	return out, nil
}

// convFrom is the caller-independent part of a Conversation.
func convFrom(c store.Conversation, members []string) Conversation {
	return Conversation{
		ID: c.ID, TeamID: c.TeamID, Name: c.Name, Kind: c.Kind, Creator: c.Creator,
		Created: c.Created, IsArchived: c.IsArchived, IsPrivate: c.IsPrivate, NumMembers: len(members),
	}
}

func peerOf(members []string, self string) string {
	for _, id := range members {
		if id != self {
			return id
		}
	}
	return self
}

func (s *Service) enrichOne(ctx context.Context, caller User, c store.Conversation) (Conversation, error) {
	out, err := s.enrich(ctx, caller, []store.Conversation{c})
	if err != nil {
		return Conversation{}, err
	}
	return out[0], nil
}

// Conversation returns one conversation as seen by caller.
func (s *Service) Conversation(ctx context.Context, caller User, id string) (Conversation, error) {
	c, err := s.st.Conversation(ctx, id)
	if err != nil {
		return Conversation{}, notFoundAs(err, ErrChannelNotFound)
	}
	if _, err := s.canSee(ctx, caller, c); err != nil {
		return Conversation{}, err
	}
	return s.enrichOne(ctx, caller, c)
}
