-- +goose Up
ALTER TABLE apps ADD COLUMN deliver_to TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE apps DROP COLUMN deliver_to;
