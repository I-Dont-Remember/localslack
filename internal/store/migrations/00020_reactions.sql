-- +goose Up
CREATE TABLE reactions (
    channel_id TEXT NOT NULL,
    ts         TEXT NOT NULL,
    name       TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    created    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (channel_id, ts, name, user_id)
);

-- +goose Down
DROP TABLE reactions;
