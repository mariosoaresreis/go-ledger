package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
	"go-ledger/internal/domain"
)

type eventStoreDB struct {
	db *bun.DB
}

// NewEventStoreRepository creates a new PostgreSQL-backed event store.
func NewEventStoreRepository(db *bun.DB) EventStoreRepository {
	return &eventStoreDB{db: db}
}

func (r *eventStoreDB) AppendEvent(ctx context.Context, event *domain.LedgerEvent) error {
	_, err := r.db.NewInsert().Model(event).Exec(ctx)
	if err != nil {
		return fmt.Errorf("event store: append event: %w", err)
	}
	return nil
}

func (r *eventStoreDB) GetEventsByAggregateID(ctx context.Context, aggregateID string) ([]*domain.LedgerEvent, error) {
	var events []*domain.LedgerEvent
	err := r.db.NewSelect().
		Model(&events).
		Where("aggregate_id = ?", aggregateID).
		OrderExpr("version ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("event store: get events: %w", err)
	}
	return events, nil
}

func (r *eventStoreDB) GetCurrentVersion(ctx context.Context, aggregateID string) (int64, error) {
	var version sql.NullInt64
	err := r.db.NewSelect().
		TableExpr("ledger_events").
		ColumnExpr("MAX(version)").
		Where("aggregate_id = ?", aggregateID).
		Scan(ctx, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("event store: get version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}
