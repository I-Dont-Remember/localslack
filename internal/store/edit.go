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
	res, err := t.tx.ExecContext(ctx,
		`UPDATE messages SET text = ?, data = ? WHERE channel_id = ? AND ts = ?`, m.Text, data, m.ChannelID, m.TS)
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("update message %s: %w", m.TS, ErrNotFound)
	}
	return nil
}
