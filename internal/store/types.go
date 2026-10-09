package store

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
}

// Conversation is a channel, DM, or group DM.
type Conversation struct {
	ID, TeamID, Name, Kind, Creator string
	Created                         int64
	IsArchived                      bool
}

// Message is a posted message; ThreadTS is empty for non-threaded messages.
type Message struct {
	ChannelID, TS, UserID, Text string
	ThreadTS                    string
	ReplyCount                  int
	LatestReply                 string
}
