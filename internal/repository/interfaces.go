package repository

import (
	"context"
	"go-ledger/internal/domain"
)

// EventStoreRepository persists immutable domain events.
type EventStoreRepository interface {
	// AppendEvent writes an event with optimistic locking (unique constraint on aggregate_id + version).
	AppendEvent(ctx context.Context, event *domain.LedgerEvent) error
	// GetEventsByAggregateID returns all events for an aggregate ordered by version ascending.
	GetEventsByAggregateID(ctx context.Context, aggregateID string) ([]*domain.LedgerEvent, error)
	// GetCurrentVersion returns the latest version number for an aggregate (0 if none).
	GetCurrentVersion(ctx context.Context, aggregateID string) (int64, error)
}

// OutboxRepository writes outbox entries in the same transaction as the event.
type OutboxRepository interface {
	InsertOutbox(ctx context.Context, entry *domain.OutboxEntry) error
}

// AccountRepository provides read access to the write-side account state (materialized from events).
type AccountRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Account, error)
	Save(ctx context.Context, account *domain.Account) error
}

// IdempotencyRepository deduplicates incoming commands by their Idempotency-Key header.
type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, record *domain.IdempotencyRecord) error
}
