package store

import (
	"context"
	"fmt"
)

// UpdateMessage rewrites a message's text and data document (blocks, subtype,
// edited marker, ...); thread columns are untouched. ErrNotFound if missing.
func (t *Tx) UpdateMessage(ctx context.Context, m Message) error {
	data, err := encodeData(m)
	if err != nil {
		return err
	}
	return t.execOne(ctx, "update message", "update message "+m.TS,
		`UPDATE messages SET text = ?, data = ? WHERE channel_id = ? AND ts = ?`, m.Text, data, m.ChannelID, m.TS)
}

// DeleteMessage removes a message row; ErrNotFound if it does not exist.
func (t *Tx) DeleteMessage(ctx context.Context, channelID, ts string) error {
	return t.execOne(ctx, "delete message", "delete message "+ts, `DELETE FROM messages WHERE channel_id = ? AND ts = ?`, channelID, ts)
}

// RecountThread recomputes a root's reply_count and latest_reply from its replies.
func (t *Tx) RecountThread(ctx context.Context, channelID, rootTS string) error {
	_, err := t.tx.ExecContext(ctx, `
		UPDATE messages SET
		  reply_count  = (SELECT count(*) FROM messages r WHERE r.channel_id = ?1 AND r.thread_ts = ?2),
		  latest_reply = COALESCE((SELECT max(r.ts) FROM messages r WHERE r.channel_id = ?1 AND r.thread_ts = ?2), '')
		WHERE channel_id = ?1 AND ts = ?2`, channelID, rootTS)
	if err != nil {
		return fmt.Errorf("recount thread: %w", err)
	}
	return nil
}
