package service

import (
	"context"
	"slices"

	"localslack/internal/store"
)

// Conversation is a channel, DM or group DM as seen by one caller.
type Conversation struct {
	ID, TeamID, Name, Kind, Creator string
	Created                         int64
	IsArchived                      bool
	// Peer is the other user of an im (the caller itself for a self-DM).
	Peer       string
	IsMember   bool
	NumMembers int
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
}

func msgFrom(m store.Message) Message {
	thread := m.ThreadTS
	if thread == "" && m.ReplyCount > 0 {
		thread = m.TS
	}
	return Message{
		ChannelID: m.ChannelID, TS: m.TS, UserID: m.UserID, Text: m.Text,
		ThreadTS: thread, ReplyCount: m.ReplyCount, LatestReply: m.LatestReply,
	}
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
		conv := Conversation{
			ID: c.ID, TeamID: c.TeamID, Name: c.Name, Kind: c.Kind, Creator: c.Creator,
			Created: c.Created, IsArchived: c.IsArchived,
			IsMember: slices.Contains(m, caller.ID), NumMembers: len(m),
		}
		if c.Kind == store.KindIM {
			conv.Peer = peerOf(m, caller.ID)
		}
		out = append(out, conv)
	}
	return out, nil
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
	return s.enrichOne(ctx, caller, c)
}

// ConversationState is a full dump of one conversation, for debugging and tests.
type ConversationState struct {
	Conversation
	Members  []string
	Messages []Message // oldest first, thread replies included
}

// Snapshot returns every conversation with its members and messages.
func (s *Service) Snapshot(ctx context.Context, caller User) ([]ConversationState, error) {
	rows, err := s.st.Conversations(ctx)
	if err != nil {
		return nil, err
	}
	convs, err := s.enrich(ctx, caller, rows)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, c := range rows {
		ids = append(ids, c.ID)
	}
	members, err := s.st.MembersOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]ConversationState, 0, len(convs))
	for _, c := range convs {
		msgs, err := s.st.Messages(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		st := ConversationState{Conversation: c, Members: members[c.ID], Messages: make([]Message, 0, len(msgs))}
		for _, m := range msgs {
			st.Messages = append(st.Messages, msgFrom(m))
		}
		out = append(out, st)
	}
	return out, nil
}
