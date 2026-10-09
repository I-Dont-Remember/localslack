-- +goose Up
CREATE TABLE teams (
    id     TEXT PRIMARY KEY,
    name   TEXT NOT NULL,
    domain TEXT NOT NULL,
    data   TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE users (
    id        TEXT PRIMARY KEY,
    team_id   TEXT NOT NULL REFERENCES teams(id),
    name      TEXT NOT NULL,
    real_name TEXT NOT NULL DEFAULT '',
    is_bot    INTEGER NOT NULL DEFAULT 0,
    data      TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    name        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL CHECK (kind IN ('channel', 'im', 'mpim')),
    creator     TEXT NOT NULL DEFAULT '',
    created     INTEGER NOT NULL DEFAULT 0,
    is_archived INTEGER NOT NULL DEFAULT 0,
    data        TEXT NOT NULL DEFAULT '{}'
);

CREATE UNIQUE INDEX conversations_channel_name
    ON conversations (lower(name)) WHERE kind = 'channel';

CREATE TABLE members (
    conversation_id TEXT NOT NULL REFERENCES conversations(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE messages (
    channel_id   TEXT NOT NULL REFERENCES conversations(id),
    ts           TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    text         TEXT NOT NULL DEFAULT '',
    thread_ts    TEXT NOT NULL DEFAULT '',
    reply_count  INTEGER NOT NULL DEFAULT 0,
    latest_reply TEXT NOT NULL DEFAULT '',
    data         TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (channel_id, ts)
);

CREATE INDEX messages_thread ON messages (channel_id, thread_ts, ts);

-- +goose Down
DROP TABLE messages;
DROP TABLE members;
DROP TABLE conversations;
DROP TABLE users;
DROP TABLE teams;
