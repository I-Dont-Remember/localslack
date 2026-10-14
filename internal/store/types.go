package store

import "encoding/json"

// Conversation kinds.
const (
	KindChannel = "channel"
	KindIM      = "im"
	KindMPIM    = "mpim"
)

// Team is a workspace.
type Team struct {
	ID, Name, Domain string
}

// User is a workspace member or bot.
type User struct {
	ID, TeamID, Name, RealName string
	IsBot                      bool
	// BotID is the B-prefixed bot identity of a bot user ("" for humans).
	BotID string
}

// Conversation is a channel, DM, or group DM.
type Conversation struct {
	ID, TeamID, Name, Kind, Creator string
	Created                         int64
	IsArchived                      bool
	IsPrivate                       bool
}

// Edit marks a message as edited by User at TS.
type Edit struct{ User, TS string }

// Message is a posted message; ThreadTS is empty for non-threaded messages.
// BotID..Edited live in the messages.data JSON column (see messages.go).
type Message struct {
	ChannelID, TS, UserID, Text string
	ThreadTS                    string
	ReplyCount                  int
	LatestReply                 string

	BotID, Subtype string
	Blocks         json.RawMessage
	Attachments    json.RawMessage
	Metadata       json.RawMessage
	Edited         *Edit
}
