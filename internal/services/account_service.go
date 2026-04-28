package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go-ledger/internal/domain"
	"go-ledger/internal/kafka"
	"go-ledger/internal/repository"
)

// CreateAccountRequest is the DTO for account creation.
type CreateAccountRequest struct {
	OwnerID  string `json:"ownerId" binding:"required"`
	Currency string `json:"currency" binding:"required,len=3"`
}

// CreditRequest is the DTO for crediting an account.
type CreditRequest struct {
	Amount    int64  `json:"amount" binding:"required,gt=0"`
	Currency  string `json:"currency" binding:"required,len=3"`
	Reference string `json:"reference" binding:"required"`
}

// DebitRequest is the DTO for debiting an account.
type DebitRequest struct {
	Amount    int64  `json:"amount" binding:"required,gt=0"`
	Currency  string `json:"currency" binding:"required,len=3"`
	Reference string `json:"reference" binding:"required"`
}

// ChangeStatusRequest is the DTO for changing account status.
type ChangeStatusRequest struct {
	Status domain.AccountStatus `json:"status" binding:"required,oneof=ACTIVE FROZEN CLOSED"`
}

// AccountCommandService handles all write-side account operations.
type AccountCommandService interface {
	CreateAccount(ctx context.Context, req CreateAccountRequest) (*domain.Account, error)
	CreditAccount(ctx context.Context, accountID string, req CreditRequest, idempotencyKey string) (*domain.LedgerEvent, error)
	DebitAccount(ctx context.Context, accountID string, req DebitRequest, idempotencyKey string) (*domain.LedgerEvent, error)
	ChangeAccountStatus(ctx context.Context, accountID string, req ChangeStatusRequest) (*domain.LedgerEvent, error)
}

type accountCommandService struct {
	db            *sqlx.DB
	eventStore    repository.EventStoreRepository
	accountRepo   repository.AccountRepository
	outboxRepo    repository.OutboxRepository
	idempotency   repository.IdempotencyRepository
	kafkaProducer *kafka.Producer
}

// NewAccountCommandService creates a new AccountCommandService.
func NewAccountCommandService(
	db *sqlx.DB,
	eventStore repository.EventStoreRepository,
	accountRepo repository.AccountRepository,
	outboxRepo repository.OutboxRepository,
	idempotency repository.IdempotencyRepository,
	kafkaProducer *kafka.Producer,
) AccountCommandService {
	return &accountCommandService{
		db:            db,
		eventStore:    eventStore,
		accountRepo:   accountRepo,
		outboxRepo:    outboxRepo,
		idempotency:   idempotency,
		kafkaProducer: kafkaProducer,
	}
}

func (s *accountCommandService) CreateAccount(ctx context.Context, req CreateAccountRequest) (*domain.Account, error) {
	account := &domain.Account{
		ID:        uuid.NewString(),
		OwnerID:   req.OwnerID,
		Currency:  req.Currency,
		Balance:   0,
		Status:    domain.StatusActive,
		Version:   1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	payload, _ := json.Marshal(domain.AccountCreatedPayload{
		AccountID: account.ID,
		OwnerID:   account.OwnerID,
		Currency:  account.Currency,
	})

	event := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: account.ID,
		Version:     1,
		EventType:   domain.EventAccountCreated,
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	outbox := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: account.ID,
		EventType:   string(domain.EventAccountCreated),
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	// Atomic: persist event + account state + outbox in one transaction.
	err := withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, event); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if err := insertAccount(ctx, tx, account); err != nil {
			return fmt.Errorf("insert account: %w", err)
		}
		if err := insertOutbox(ctx, tx, outbox); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create account: transaction: %w", err)
	}

	// Best-effort publish to Kafka (outbox relay is the reliable path).
	_ = s.kafkaProducer.PublishEvent(ctx, event)

	return account, nil
}

func (s *accountCommandService) CreditAccount(ctx context.Context, accountID string, req CreditRequest, idempotencyKey string) (*domain.LedgerEvent, error) {
	// Check idempotency.
	if idempotencyKey != "" {
		rec, err := s.idempotency.Get(ctx, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			var evt domain.LedgerEvent
			_ = json.Unmarshal(rec.Response, &evt)
			return &evt, nil
		}
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.Status != domain.StatusActive {
		return nil, fmt.Errorf("account %s is not active", accountID)
	}
	if account.Currency != req.Currency {
		return nil, fmt.Errorf("currency mismatch: account is %s, request is %s", account.Currency, req.Currency)
	}

	currentVersion, err := s.eventStore.GetCurrentVersion(ctx, accountID)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(domain.AccountCreditedPayload{
		AccountID: accountID,
		Amount:    req.Amount,
		Currency:  req.Currency,
		Reference: req.Reference,
	})

	event := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		Version:     currentVersion + 1,
		EventType:   domain.EventAccountCredited,
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	outbox := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		EventType:   string(domain.EventAccountCredited),
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	account.Balance += req.Amount
	account.Version = event.Version
	account.UpdatedAt = time.Now().UTC()

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, event); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if err := insertOutbox(ctx, tx, outbox); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if err := updateAccount(ctx, tx, account); err != nil {
			return fmt.Errorf("update account: %w", err)
		}
		if idempotencyKey != "" {
			evtBytes, _ := json.Marshal(event)
			rec := &domain.IdempotencyRecord{
				Key:        idempotencyKey,
				StatusCode: 201,
				Response:   evtBytes,
				CreatedAt:  time.Now().UTC(),
			}
			if err := insertIdempotency(ctx, tx, rec); err != nil {
				return fmt.Errorf("insert idempotency: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("credit account: transaction: %w", err)
	}

	_ = s.kafkaProducer.PublishEvent(ctx, event)
	return event, nil
}

func (s *accountCommandService) DebitAccount(ctx context.Context, accountID string, req DebitRequest, idempotencyKey string) (*domain.LedgerEvent, error) {
	// Check idempotency.
	if idempotencyKey != "" {
		rec, err := s.idempotency.Get(ctx, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			var evt domain.LedgerEvent
			_ = json.Unmarshal(rec.Response, &evt)
			return &evt, nil
		}
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.Status != domain.StatusActive {
		return nil, fmt.Errorf("account %s is not active", accountID)
	}
	if account.Currency != req.Currency {
		return nil, fmt.Errorf("currency mismatch: account is %s, request is %s", account.Currency, req.Currency)
	}
	if account.Balance < req.Amount {
		return nil, fmt.Errorf("insufficient balance: available %d, requested %d", account.Balance, req.Amount)
	}

	currentVersion, err := s.eventStore.GetCurrentVersion(ctx, accountID)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(domain.AccountDebitedPayload{
		AccountID: accountID,
		Amount:    req.Amount,
		Currency:  req.Currency,
		Reference: req.Reference,
	})

	event := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		Version:     currentVersion + 1,
		EventType:   domain.EventAccountDebited,
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	outbox := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		EventType:   string(domain.EventAccountDebited),
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	account.Balance -= req.Amount
	account.Version = event.Version
	account.UpdatedAt = time.Now().UTC()

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, event); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if err := insertOutbox(ctx, tx, outbox); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if err := updateAccount(ctx, tx, account); err != nil {
			return fmt.Errorf("update account: %w", err)
		}
		if idempotencyKey != "" {
			evtBytes, _ := json.Marshal(event)
			rec := &domain.IdempotencyRecord{
				Key:        idempotencyKey,
				StatusCode: 201,
				Response:   evtBytes,
				CreatedAt:  time.Now().UTC(),
			}
			if err := insertIdempotency(ctx, tx, rec); err != nil {
				return fmt.Errorf("insert idempotency: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("debit account: transaction: %w", err)
	}

	_ = s.kafkaProducer.PublishEvent(ctx, event)
	return event, nil
}

func (s *accountCommandService) ChangeAccountStatus(ctx context.Context, accountID string, req ChangeStatusRequest) (*domain.LedgerEvent, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	oldStatus := account.Status
	payload, _ := json.Marshal(domain.AccountStatusChangedPayload{
		AccountID: accountID,
		OldStatus: oldStatus,
		NewStatus: req.Status,
	})

	currentVersion, err := s.eventStore.GetCurrentVersion(ctx, accountID)
	if err != nil {
		return nil, err
	}

	event := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		Version:     currentVersion + 1,
		EventType:   domain.EventAccountStatusChanged,
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	outbox := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: accountID,
		EventType:   string(domain.EventAccountStatusChanged),
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	account.Status = req.Status
	account.Version = event.Version
	account.UpdatedAt = time.Now().UTC()

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, event); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if err := insertOutbox(ctx, tx, outbox); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if err := updateAccount(ctx, tx, account); err != nil {
			return fmt.Errorf("update account: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("change status: transaction: %w", err)
	}

	_ = s.kafkaProducer.PublishEvent(ctx, event)
	return event, nil
}

func withTx(ctx context.Context, db *sqlx.DB, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func insertLedgerEvent(ctx context.Context, tx *sqlx.Tx, event *domain.LedgerEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_events (id, aggregate_id, version, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.AggregateID, event.Version, event.EventType, event.Payload, event.CreatedAt)
	return err
}

func insertAccount(ctx context.Context, tx *sqlx.Tx, account *domain.Account) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (id, owner_id, currency, balance, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, account.ID, account.OwnerID, account.Currency, account.Balance, account.Status, account.Version, account.CreatedAt, account.UpdatedAt)
	return err
}

func updateAccount(ctx context.Context, tx *sqlx.Tx, account *domain.Account) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET balance = $1, status = $2, version = $3, updated_at = $4
		WHERE id = $5
	`, account.Balance, account.Status, account.Version, account.UpdatedAt, account.ID)
	return err
}

func insertOutbox(ctx context.Context, tx *sqlx.Tx, outbox *domain.OutboxEntry) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (id, aggregate_id, event_type, payload, created_at, processed)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, outbox.ID, outbox.AggregateID, outbox.EventType, outbox.Payload, outbox.CreatedAt, outbox.Processed)
	return err
}

func insertIdempotency(ctx context.Context, tx *sqlx.Tx, rec *domain.IdempotencyRecord) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, status_code, response, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key) DO NOTHING
	`, rec.Key, rec.StatusCode, rec.Response, rec.CreatedAt)
	return err
}
