-- +goose Up
-- Backstop for service-level checks. Pre-existing duplicates (never allowed by
-- the service, but possible in hand-edited DBs) are made distinct first so the
-- index cannot block startup.
UPDATE apps SET bot_token = bot_token || '-dup-' || id
 WHERE bot_token <> '' AND rowid NOT IN (SELECT MIN(rowid) FROM apps WHERE bot_token <> '' GROUP BY bot_token);
UPDATE apps SET app_token = app_token || '-dup-' || id
 WHERE app_token <> '' AND rowid NOT IN (SELECT MIN(rowid) FROM apps WHERE app_token <> '' GROUP BY app_token);
CREATE UNIQUE INDEX apps_bot_token_unique ON apps(bot_token) WHERE bot_token <> '';
CREATE UNIQUE INDEX apps_app_token_unique ON apps(app_token) WHERE app_token <> '';

-- +goose Down
DROP INDEX apps_app_token_unique;
DROP INDEX apps_bot_token_unique;
