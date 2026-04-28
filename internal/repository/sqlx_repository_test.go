package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
)

func TestAccountRepository_GetByID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	db := sqlx.NewDb(sqlDB, "sqlmock")
	repo := NewAccountRepository(db)

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, owner_id, currency, balance, status, version, created_at, updated_at
		FROM accounts
		WHERE id = $1
	`)).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "currency", "balance", "status", "version", "created_at", "updated_at"}).
			AddRow("acc-1", "owner-1", "USD", int64(2500), "ACTIVE", int64(3), now, now))

	acc, err := repo.GetByID(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if acc.ID != "acc-1" || acc.Balance != 2500 || acc.Status != domain.StatusActive {
		t.Fatalf("unexpected account: %+v", acc)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEventStoreRepository_GetCurrentVersion(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	db := sqlx.NewDb(sqlDB, "sqlmock")
	repo := NewEventStoreRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT MAX(version)
		FROM ledger_events
		WHERE aggregate_id = $1
	`)).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(9)))

	version, err := repo.GetCurrentVersion(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("get current version: %v", err)
	}
	if version != 9 {
		t.Fatalf("unexpected version: got %d want %d", version, 9)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
