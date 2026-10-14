package store

import (
	"context"
)

// Reaction is one user's emoji on one message.
type Reaction struct {
	ChannelID, TS, Name, UserID string
	Created                     int64
}

const reactionCols = `channel_id, ts, name, user_id, created`

func scanReaction(r scanner, v *Reaction) error {
	return r.Scan(&v.ChannelID, &v.TS, &v.Name, &v.UserID, &v.Created)
}

// InsertReaction adds a reaction; the same user, emoji and message again yields ErrDuplicate.
func (t *Tx) InsertReaction(ctx context.Context, r Reaction) error {
	return t.exec(ctx, "insert reaction",
		`INSERT INTO reactions (`+reactionCols+`) VALUES (?, ?, ?, ?, ?)`,
		r.ChannelID, r.TS, r.Name, r.UserID, r.Created)
}

// DeleteReactions removes every reaction on a message.
func (t *Tx) DeleteReactions(ctx context.Context, channelID, ts string) error {
	return t.exec(ctx, "delete reactions", `DELETE FROM reactions WHERE channel_id = ? AND ts = ?`, channelID, ts)
}

// ReactionsOf returns a message's reactions in the order they were added, seeing this txn's writes.
func (t *Tx) ReactionsOf(ctx context.Context, channelID, ts string) ([]Reaction, error) {
	return query(ctx, t.tx, "reactions",
		`SELECT `+reactionCols+` FROM reactions WHERE channel_id = ? AND ts = ? ORDER BY created, rowid`,
		scanReaction, channelID, ts)
}

// ReactionsFor returns reactions for the given messages of one channel, keyed by ts,
// each in the order they were added. Messages without reactions are absent.
func (s *Store) ReactionsFor(ctx context.Context, channelID string, tss []string) (map[string][]Reaction, error) {
	out := map[string][]Reaction{}
	if len(tss) == 0 {
		return out, nil
	}
	args := []any{channelID}
	for _, ts := range tss {
		args = append(args, ts)
	}
	rs, err := query(ctx, s.reader, "reactions",
		`SELECT `+reactionCols+` FROM reactions WHERE channel_id = ? AND ts IN (`+placeholders(len(tss))+`) ORDER BY created, rowid`,
		scanReaction, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		out[r.TS] = append(out[r.TS], r)
	}
	return out, nil
}
