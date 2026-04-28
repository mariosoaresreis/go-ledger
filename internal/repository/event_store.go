package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
)

type eventStoreDB struct {
	db *sqlx.DB
}

// NewEventStoreRepository creates a new PostgreSQL-backed event store.
func NewEventStoreRepository(db *sqlx.DB) EventStoreRepository {
	return &eventStoreDB{db: db}
}

func (r *eventStoreDB) AppendEvent(ctx context.Context, event *domain.LedgerEvent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ledger_events (id, aggregate_id, version, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.AggregateID, event.Version, event.EventType, event.Payload, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("event store: append event: %w", err)
	}
	return nil
}

func (r *eventStoreDB) GetEventsByAggregateID(ctx context.Context, aggregateID string) ([]*domain.LedgerEvent, error) {
	var events []*domain.LedgerEvent
	err := r.db.SelectContext(ctx, &events, `
		SELECT id, aggregate_id, version, event_type, payload, created_at
		FROM ledger_events
		WHERE aggregate_id = $1
		ORDER BY version ASC
	`, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("event store: get events: %w", err)
	}
	return events, nil
}

func (r *eventStoreDB) GetCurrentVersion(ctx context.Context, aggregateID string) (int64, error) {
	var version sql.NullInt64
	err := r.db.GetContext(ctx, &version, `
		SELECT MAX(version)
		FROM ledger_events
		WHERE aggregate_id = $1
	`, aggregateID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("event store: get version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}
