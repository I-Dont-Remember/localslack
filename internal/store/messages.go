package store

import (
	"context"
	"encoding/json"
	"fmt"
)

const msgCols = `channel_id, ts, user_id, text, thread_ts, reply_count, latest_reply, data`

// messageData is the messages.data JSON document: everything beyond the core columns.
type messageData struct {
	BotID       string          `json:"bot_id,omitempty"`
	Subtype     string          `json:"subtype,omitempty"`
	Blocks      json.RawMessage `json:"blocks,omitempty"`
	Attachments json.RawMessage `json:"attachments,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Edited      *Edit           `json:"edited,omitempty"`
}

func encodeData(m Message) (string, error) {
	b, err := json.Marshal(messageData{
		BotID: m.BotID, Subtype: m.Subtype, Blocks: m.Blocks, Attachments: m.Attachments,
		Metadata: m.Metadata, Edited: m.Edited,
	})
	if err != nil {
		return "", fmt.Errorf("encode message data: %w", err)
	}
	return string(b), nil
}

func decodeData(raw string, m *Message) error {
	var d messageData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return fmt.Errorf("decode message data: %w", err)
	}
	m.BotID, m.Subtype, m.Blocks, m.Attachments, m.Metadata, m.Edited =
		d.BotID, d.Subtype, d.Blocks, d.Attachments, d.Metadata, d.Edited
	return nil
}

func scanMsg(r scanner, m *Message) error {
	var data string
	if err := r.Scan(&m.ChannelID, &m.TS, &m.UserID, &m.Text, &m.ThreadTS, &m.ReplyCount, &m.LatestReply, &data); err != nil {
		return err
	}
	return decodeData(data, m)
}

// InsertMessage adds a message.
func (t *Tx) InsertMessage(ctx context.Context, m Message) error {
	data, err := encodeData(m)
	if err != nil {
		return err
	}
	return t.exec(ctx, "insert message",
		`INSERT INTO messages (channel_id, ts, user_id, text, thread_ts, reply_count, latest_reply, data) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ChannelID, m.TS, m.UserID, m.Text, m.ThreadTS, m.ReplyCount, m.LatestReply, data)
}

// Message returns a message by key, seeing this txn's writes.
func (t *Tx) Message(ctx context.Context, channelID, ts string) (Message, error) {
	return messageByTS(ctx, t.tx, channelID, ts)
}

// BumpThread records a new reply on a thread parent; ErrNotFound if the parent is missing.
func (t *Tx) BumpThread(ctx context.Context, channelID, parentTS, replyTS string) error {
	return t.execOne(ctx, "bump thread", "bump thread: parent "+parentTS,
		`UPDATE messages SET reply_count = reply_count + 1, latest_reply = ? WHERE channel_id = ? AND ts = ?`,
		replyTS, channelID, parentTS)
}

// Message returns a message by key.
func (s *Store) Message(ctx context.Context, channelID, ts string) (Message, error) {
	return messageByTS(ctx, s.reader, channelID, ts)
}

// Messages returns every message in a channel, oldest first (replies included).
func (s *Store) Messages(ctx context.Context, channelID string) ([]Message, error) {
	return query(ctx, s.reader, "messages",
		`SELECT `+msgCols+` FROM messages WHERE channel_id = ? ORDER BY ts`, scanMsg, channelID)
}

// AllMessages returns every message in every channel, ordered by channel then ts.
func (s *Store) AllMessages(ctx context.Context) ([]Message, error) {
	return query(ctx, s.reader, "messages",
		`SELECT `+msgCols+` FROM messages ORDER BY channel_id, ts`, scanMsg)
}

func messageByTS(ctx context.Context, q queryer, channelID, ts string) (Message, error) {
	return getOne(ctx, q, "message", `SELECT `+msgCols+` FROM messages WHERE channel_id = ? AND ts = ?`, scanMsg, channelID, ts)
}

// HistoryQuery selects top-level messages. Zero values mean "no bound".
// Before is an exclusive keyset cursor; Oldest/Latest are Slack range bounds
// (exclusive unless Inclusive). Limit 0 means unlimited.
type HistoryQuery struct {
	Before, Oldest, Latest string
	Inclusive              bool
	Limit                  int
}

// HistoryPage returns top-level messages (thread replies excluded), newest first.
func (s *Store) HistoryPage(ctx context.Context, channelID string, hq HistoryQuery) ([]Message, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE channel_id = ? AND thread_ts = ''`
	args := []any{channelID}
	lo, hi := ">", "<"
	if hq.Inclusive {
		lo, hi = ">=", "<="
	}
	if hq.Before != "" {
		q += ` AND ts < ?`
		args = append(args, hq.Before)
	}
	if hq.Latest != "" {
		q += ` AND ts ` + hi + ` ?`
		args = append(args, hq.Latest)
	}
	if hq.Oldest != "" {
		q += ` AND ts ` + lo + ` ?`
		args = append(args, hq.Oldest)
	}
	q, args = withLimit(q+` ORDER BY ts DESC`, args, hq.Limit)
	return query(ctx, s.reader, "history page", q, scanMsg, args...)
}

// ThreadPage returns a thread's parent followed by its replies, oldest first.
// afterTS is an exclusive keyset cursor; limit 0 means unlimited.
func (s *Store) ThreadPage(ctx context.Context, channelID, threadTS, afterTS string, limit int) ([]Message, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE channel_id = ? AND (ts = ? OR thread_ts = ?)`
	args := []any{channelID, threadTS, threadTS}
	if afterTS != "" {
		q += ` AND ts > ?`
		args = append(args, afterTS)
	}
	q, args = withLimit(q+` ORDER BY ts`, args, limit)
	return query(ctx, s.reader, "thread page", q, scanMsg, args...)
}
