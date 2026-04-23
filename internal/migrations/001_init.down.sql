-- +migrate Down
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS ledger_events;
DROP TABLE IF EXISTS accounts;

