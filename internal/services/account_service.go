package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
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
	db            *bun.DB
	eventStore    repository.EventStoreRepository
	accountRepo   repository.AccountRepository
	outboxRepo    repository.OutboxRepository
	idempotency   repository.IdempotencyRepository
	kafkaProducer *kafka.Producer
}

// NewAccountCommandService creates a new AccountCommandService.
func NewAccountCommandService(
	db *bun.DB,
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
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(event).Exec(ctx); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if _, err := tx.NewInsert().Model(account).Exec(ctx); err != nil {
			return fmt.Errorf("insert account: %w", err)
		}
		if _, err := tx.NewInsert().Model(outbox).Exec(ctx); err != nil {
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

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(event).Exec(ctx); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if _, err := tx.NewInsert().Model(outbox).Exec(ctx); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if _, err := tx.NewUpdate().Model(account).WherePK().Exec(ctx); err != nil {
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
			if _, err := tx.NewInsert().Model(rec).On("CONFLICT (key) DO NOTHING").Exec(ctx); err != nil {
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

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(event).Exec(ctx); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if _, err := tx.NewInsert().Model(outbox).Exec(ctx); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if _, err := tx.NewUpdate().Model(account).WherePK().Exec(ctx); err != nil {
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
			if _, err := tx.NewInsert().Model(rec).On("CONFLICT (key) DO NOTHING").Exec(ctx); err != nil {
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

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(event).Exec(ctx); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		if _, err := tx.NewInsert().Model(outbox).Exec(ctx); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}
		if _, err := tx.NewUpdate().Model(account).WherePK().Exec(ctx); err != nil {
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
