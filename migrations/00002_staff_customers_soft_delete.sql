-- +goose Up
-- +goose StatementBegin

ALTER TABLE staff ADD COLUMN deleted_at TIMESTAMP;
ALTER TABLE customers ADD COLUMN deleted_at TIMESTAMP;

UPDATE staff SET role = 'cashier' WHERE role = 'shift_lead';
UPDATE staff SET login = LOWER(TRIM(login));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE customers DROP COLUMN deleted_at;
ALTER TABLE staff DROP COLUMN deleted_at;

-- +goose StatementEnd
