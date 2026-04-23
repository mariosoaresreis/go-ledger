package repository

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	"go-ledger/internal/domain"
)

type outboxDB struct {
	db *bun.DB
}

// NewOutboxRepository creates a new PostgreSQL-backed outbox repository.
func NewOutboxRepository(db *bun.DB) OutboxRepository {
	return &outboxDB{db: db}
}

func (r *outboxDB) InsertOutbox(ctx context.Context, entry *domain.OutboxEntry) error {
	_, err := r.db.NewInsert().Model(entry).Exec(ctx)
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
	return nil
}
