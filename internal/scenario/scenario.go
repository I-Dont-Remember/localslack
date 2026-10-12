// Package scenario defines the Fixture: the one JSON shape shared by seeding,
// --scenario files, reset, snapshots and diff (docs/TESTING_API.md). Leaf
// package: no service or store imports; the service consumes Fixture values.
package scenario

import "encoding/json"

// Version is the only supported fixture version.
const Version = 1

// Fixture is a complete workspace state, or a recipe (Generate) for one.
type Fixture struct {
	Version  int       `json:"version"`
	Team     *Team     `json:"team,omitempty"`
	Users    []User    `json:"users,omitempty"`
	Channels []Channel `json:"channels,omitempty"`
	DMs      []DM      `json:"dms,omitempty"`
	Messages []Message `json:"messages,omitempty"`
	Generate *Generate `json:"generate,omitempty"`
}

// Team is the workspace.
type Team struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// User is a workspace member; app bot users are excluded from snapshots.
type User struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	RealName string `json:"real_name,omitempty"`
	IsBot    bool   `json:"is_bot,omitempty"`
	BotID    string `json:"bot_id,omitempty"`
}

// Channel is a public or private channel with its members (user ids).
type Channel struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Creator    string   `json:"creator,omitempty"`
	Created    int64    `json:"created,omitempty"`
	IsArchived bool     `json:"is_archived,omitempty"`
	IsPrivate  bool     `json:"is_private,omitempty"`
	Members    []string `json:"members,omitempty"`
}

// DM is a direct or group message between Members.
type DM struct {
	ID      string   `json:"id,omitempty"`
	Members []string `json:"members"`
}

// Edit marks a message as edited.
type Edit struct {
	User string `json:"user"`
	TS   string `json:"ts"`
}

// Reaction is an emoji on a message.
type Reaction struct {
	Name  string   `json:"name"`
	Users []string `json:"users"`
}

// Message is one message in Channel (a channel or DM id, or name in hand-written files).
type Message struct {
	Channel     string          `json:"channel"`
	TS          string          `json:"ts,omitempty"`
	User        string          `json:"user"`
	Text        string          `json:"text"`
	ThreadTS    string          `json:"thread_ts,omitempty"`
	BotID       string          `json:"bot_id,omitempty"`
	Subtype     string          `json:"subtype,omitempty"`
	Blocks      json.RawMessage `json:"blocks,omitempty"`
	Attachments json.RawMessage `json:"attachments,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Edited      *Edit           `json:"edited,omitempty"`
	Reactions   []Reaction      `json:"reactions,omitempty"`
}

// Generate asks for synthetic data, deterministic per Seed.
type Generate struct {
	Seed               string  `json:"seed,omitempty"`
	Users              int     `json:"users,omitempty"`
	Channels           int     `json:"channels,omitempty"`
	MessagesPerChannel int     `json:"messages_per_channel,omitempty"`
	ThreadRatio        float64 `json:"thread_ratio,omitempty"`
	PrivateChannels    int     `json:"private_channels,omitempty"`
}
