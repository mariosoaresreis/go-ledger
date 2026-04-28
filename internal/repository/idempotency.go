package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
)

type idempotencyDB struct {
	db *sqlx.DB
}

// NewIdempotencyRepository creates a new PostgreSQL-backed idempotency repository.
func NewIdempotencyRepository(db *sqlx.DB) IdempotencyRepository {
	return &idempotencyDB{db: db}
}

func (r *idempotencyDB) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	record := &domain.IdempotencyRecord{}
	err := r.db.GetContext(ctx, record, `
		SELECT key, status_code, response, created_at
		FROM idempotency_keys
		WHERE key = $1
	`, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // not found - new request
		}
		return nil, fmt.Errorf("idempotency: get: %w", err)
	}
	return record, nil
}

func (r *idempotencyDB) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, status_code, response, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key) DO NOTHING
	`, record.Key, record.StatusCode, record.Response, record.CreatedAt)
	if err != nil {
		return fmt.Errorf("idempotency: save: %w", err)
	}
	return nil
}
