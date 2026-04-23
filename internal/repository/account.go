package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
	"go-ledger/internal/domain"
)

type accountDB struct {
	db *bun.DB
}

// NewAccountRepository creates a new PostgreSQL-backed account repository.
func NewAccountRepository(db *bun.DB) AccountRepository {
	return &accountDB{db: db}
}

func (r *accountDB) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	account := &domain.Account{}
	err := r.db.NewSelect().Model(account).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("account %s not found", id)
		}
		return nil, fmt.Errorf("account repository: get by id: %w", err)
	}
	return account, nil
}

func (r *accountDB) Save(ctx context.Context, account *domain.Account) error {
	_, err := r.db.NewInsert().Model(account).
		On("CONFLICT (id) DO UPDATE").
		Set("balance = EXCLUDED.balance").
		Set("status = EXCLUDED.status").
		Set("version = EXCLUDED.version").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("account repository: save: %w", err)
	}
	return nil
}
