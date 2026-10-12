-- +goose Up
CREATE TABLE apps (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL DEFAULT '',
    manifest           TEXT NOT NULL DEFAULT '{}',
    signing_secret     TEXT NOT NULL DEFAULT '',
    bot_token          TEXT NOT NULL DEFAULT '',
    app_token          TEXT NOT NULL DEFAULT '',
    verification_token TEXT NOT NULL DEFAULT '',
    request_url        TEXT NOT NULL DEFAULT '',
    bot_events         TEXT NOT NULL DEFAULT '[]',
    scopes             TEXT NOT NULL DEFAULT '[]',
    socket_mode        INTEGER NOT NULL DEFAULT 0,
    verified           INTEGER NOT NULL DEFAULT 0,
    bot_user_id        TEXT NOT NULL DEFAULT '',
    bot_id             TEXT NOT NULL DEFAULT '',
    created            INTEGER NOT NULL DEFAULT 0
);

ALTER TABLE users ADD COLUMN bot_id TEXT NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN is_private INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE conversations DROP COLUMN is_private;
ALTER TABLE users DROP COLUMN bot_id;
DROP TABLE apps;
