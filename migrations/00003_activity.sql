-- +goose Up
-- +goose StatementBegin

CREATE TABLE activity (
    id             TEXT PRIMARY KEY,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_staff_id TEXT NOT NULL DEFAULT '',
    actor_name     TEXT NOT NULL DEFAULT '',
    store_id       TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL,
    title          TEXT NOT NULL,
    detail         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX activity_created_at_idx ON activity(created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE activity;

-- +goose StatementEnd
