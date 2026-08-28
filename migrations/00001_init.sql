-- +goose Up
-- +goose StatementBegin

CREATE TABLE stores (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    address    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE staff (
    id            TEXT PRIMARY KEY,
    store_id      TEXT NOT NULL REFERENCES stores(id),
    login         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    pin_hash      TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL,
    active        INTEGER NOT NULL DEFAULT 1,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE customers (
    id               TEXT PRIMARY KEY,
    barcode          TEXT NOT NULL UNIQUE,
    display_name     TEXT NOT NULL DEFAULT '',
    phone            TEXT NOT NULL DEFAULT '',
    apple_auth_token TEXT NOT NULL,
    google_object_id TEXT NOT NULL DEFAULT '',
    points           INTEGER NOT NULL DEFAULT 0,
    blocked          INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX customers_phone_uq
    ON customers(phone)
    WHERE phone IS NOT NULL AND phone != '';

CREATE TABLE apple_devices (
    id                TEXT PRIMARY KEY,
    customer_id       TEXT NOT NULL REFERENCES customers(id),
    device_library_id TEXT NOT NULL,
    push_token        TEXT NOT NULL,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (device_library_id, customer_id)
);

CREATE TABLE receipts (
    id                   TEXT PRIMARY KEY,
    store_id             TEXT NOT NULL REFERENCES stores(id),
    staff_id             TEXT NOT NULL REFERENCES staff(id),
    customer_id          TEXT NOT NULL REFERENCES customers(id),
    amount_rub           INTEGER NOT NULL,
    redeem_points        INTEGER NOT NULL DEFAULT 0,
    earn_points          INTEGER NOT NULL DEFAULT 0,
    status               TEXT NOT NULL,
    points_after         INTEGER NOT NULL DEFAULT 0,
    points_after_refund  INTEGER,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    refunded_at          TIMESTAMP
);

CREATE TABLE ledger (
    id             TEXT PRIMARY KEY,
    customer_id    TEXT NOT NULL REFERENCES customers(id),
    receipt_id     TEXT REFERENCES receipts(id),
    delta          INTEGER NOT NULL,
    reason         TEXT NOT NULL,
    actor_staff_id TEXT,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX ledger_customer_idx ON ledger(customer_id, created_at);

CREATE TABLE job_dedupe (
    key        TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE wallet_jobs (
    id           TEXT PRIMARY KEY,
    customer_id  TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'wallet_update',
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP
);

CREATE INDEX wallet_jobs_pending_idx ON wallet_jobs(processed_at, created_at);

CREATE TABLE loyalty_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE staff_sessions (
    jti        TEXT PRIMARY KEY,
    staff_id   TEXT NOT NULL REFERENCES staff(id),
    kind       TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked    INTEGER NOT NULL DEFAULT 0
);

INSERT INTO loyalty_settings (key, value) VALUES
    ('earn_percent', '5'),
    ('earn_round', 'down'),
    ('earn_base', 'after_store_discount_before_points'),
    ('earn_min_receipt', '0'),
    ('redeem_rate', '1'),
    ('redeem_min', '100'),
    ('redeem_max_share', '50'),
    ('expire_days', '');

INSERT INTO stores (id, name, address) VALUES
    ('00000000-0000-4000-8000-000000000001', 'MERCH', '');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS staff_sessions;
DROP TABLE IF EXISTS loyalty_settings;
DROP TABLE IF EXISTS wallet_jobs;
DROP TABLE IF EXISTS job_dedupe;
DROP TABLE IF EXISTS ledger;
DROP TABLE IF EXISTS receipts;
DROP TABLE IF EXISTS apple_devices;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS staff;
DROP TABLE IF EXISTS stores;
-- +goose StatementEnd
