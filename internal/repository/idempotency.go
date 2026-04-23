package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
	"go-ledger/internal/domain"
)

type idempotencyDB struct {
	db *bun.DB
}

// NewIdempotencyRepository creates a new PostgreSQL-backed idempotency repository.
func NewIdempotencyRepository(db *bun.DB) IdempotencyRepository {
	return &idempotencyDB{db: db}
}

func (r *idempotencyDB) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	record := &domain.IdempotencyRecord{}
	err := r.db.NewSelect().Model(record).Where("key = ?", key).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // not found – new request
		}
		return nil, fmt.Errorf("idempotency: get: %w", err)
	}
	return record, nil
}

func (r *idempotencyDB) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	_, err := r.db.NewInsert().Model(record).
		On("CONFLICT (key) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("idempotency: save: %w", err)
	}
	return nil
}
