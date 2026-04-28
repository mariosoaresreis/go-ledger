package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
)

type outboxDB struct {
	db *sqlx.DB
}

// NewOutboxRepository creates a new PostgreSQL-backed outbox repository.
func NewOutboxRepository(db *sqlx.DB) OutboxRepository {
	return &outboxDB{db: db}
}

func (r *outboxDB) InsertOutbox(ctx context.Context, entry *domain.OutboxEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO outbox (id, aggregate_id, event_type, payload, created_at, processed)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entry.ID, entry.AggregateID, entry.EventType, entry.Payload, entry.CreatedAt, entry.Processed)
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
	return nil
}
