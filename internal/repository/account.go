package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
)

type accountDB struct {
	db *sqlx.DB
}

// NewAccountRepository creates a new PostgreSQL-backed account repository.
func NewAccountRepository(db *sqlx.DB) AccountRepository {
	return &accountDB{db: db}
}

func (r *accountDB) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	account := &domain.Account{}
	err := r.db.GetContext(ctx, account, `
		SELECT id, owner_id, currency, balance, status, version, created_at, updated_at
		FROM accounts
		WHERE id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("account %s not found", id)
		}
		return nil, fmt.Errorf("account repository: get by id: %w", err)
	}
	return account, nil
}

func (r *accountDB) Save(ctx context.Context, account *domain.Account) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO accounts (id, owner_id, currency, balance, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE
		SET balance = EXCLUDED.balance,
			status = EXCLUDED.status,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at
	`,
		account.ID,
		account.OwnerID,
		account.Currency,
		account.Balance,
		account.Status,
		account.Version,
		account.CreatedAt,
		account.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("account repository: save: %w", err)
	}
	return nil
}
