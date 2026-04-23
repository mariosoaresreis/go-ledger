-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Write-side account state (materialized from events for fast validation)
CREATE TABLE IF NOT EXISTS accounts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id    UUID        NOT NULL,
    currency    CHAR(3)     NOT NULL,
    balance     BIGINT      NOT NULL DEFAULT 0,  -- stored as minor units (cents)
    status      VARCHAR(10) NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE','FROZEN','CLOSED')),
    version     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_accounts_owner ON accounts(owner_id);

-- Immutable event store
CREATE TABLE IF NOT EXISTS ledger_events (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    aggregate_id UUID        NOT NULL,
    version      BIGINT      NOT NULL,
    event_type   VARCHAR(50) NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Optimistic locking: only one event per version per aggregate
    CONSTRAINT uq_ledger_events_aggregate_version UNIQUE (aggregate_id, version)
);

CREATE INDEX IF NOT EXISTS idx_ledger_events_aggregate ON ledger_events(aggregate_id);
CREATE INDEX IF NOT EXISTS idx_ledger_events_type      ON ledger_events(event_type);
CREATE INDEX IF NOT EXISTS idx_ledger_events_created   ON ledger_events(created_at);

-- Transactional outbox (Debezium/CDC relay to Kafka)
CREATE TABLE IF NOT EXISTS outbox (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    aggregate_id UUID        NOT NULL,
    event_type   VARCHAR(50) NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed    BOOLEAN     NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_outbox_processed ON outbox(processed) WHERE processed = FALSE;

-- Idempotency deduplication table
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key         VARCHAR(255) PRIMARY KEY,
    status_code INT          NOT NULL,
    response    JSONB,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Auto-expire idempotency records after 24 h (run periodically via pg_cron or a job)
-- CREATE INDEX IF NOT EXISTS idx_idempotency_created ON idempotency_keys(created_at);

