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

// TransferRequest is the DTO for initiating a transfer.
type TransferRequest struct {
	SourceAccountID string `json:"sourceAccountId" binding:"required,uuid"`
	TargetAccountID string `json:"targetAccountId" binding:"required,uuid"`
	Amount          int64  `json:"amount" binding:"required,gt=0"`
	Currency        string `json:"currency" binding:"required,len=3"`
}

// TransferCommandService handles transfer saga orchestration.
type TransferCommandService interface {
	InitiateTransfer(ctx context.Context, req TransferRequest, idempotencyKey string) (*domain.LedgerEvent, error)
}

type transferCommandService struct {
	db            *bun.DB
	eventStore    repository.EventStoreRepository
	accountRepo   repository.AccountRepository
	outboxRepo    repository.OutboxRepository
	idempotency   repository.IdempotencyRepository
	kafkaProducer *kafka.Producer
}

// NewTransferCommandService creates a new TransferCommandService.
func NewTransferCommandService(
	db *bun.DB,
	eventStore repository.EventStoreRepository,
	accountRepo repository.AccountRepository,
	outboxRepo repository.OutboxRepository,
	idempotency repository.IdempotencyRepository,
	kafkaProducer *kafka.Producer,
) TransferCommandService {
	return &transferCommandService{
		db:            db,
		eventStore:    eventStore,
		accountRepo:   accountRepo,
		outboxRepo:    outboxRepo,
		idempotency:   idempotency,
		kafkaProducer: kafkaProducer,
	}
}

// InitiateTransfer begins a choreography-based saga:
//  1. Validates source & target accounts.
//  2. Persists TRANSFER_INITIATED + ACCOUNT_DEBITED (source) atomically.
//  3. The Event Processor consumes ACCOUNT_DEBITED → emits ACCOUNT_CREDITED (target).
func (s *transferCommandService) InitiateTransfer(ctx context.Context, req TransferRequest, idempotencyKey string) (*domain.LedgerEvent, error) {
	// Idempotency check.
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

	src, err := s.accountRepo.GetByID(ctx, req.SourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("source account: %w", err)
	}
	if src.Status != domain.StatusActive {
		return nil, fmt.Errorf("source account %s is not active", req.SourceAccountID)
	}
	if src.Currency != req.Currency {
		return nil, fmt.Errorf("currency mismatch on source account")
	}
	if src.Balance < req.Amount {
		return nil, fmt.Errorf("insufficient balance: available %d, requested %d", src.Balance, req.Amount)
	}

	tgt, err := s.accountRepo.GetByID(ctx, req.TargetAccountID)
	if err != nil {
		return nil, fmt.Errorf("target account: %w", err)
	}
	if tgt.Status != domain.StatusActive {
		return nil, fmt.Errorf("target account %s is not active", req.TargetAccountID)
	}

	transferID := uuid.NewString()
	now := time.Now().UTC()

	// TRANSFER_INITIATED event on the transfer aggregate.
	transferPayload, _ := json.Marshal(domain.TransferInitiatedPayload{
		TransferID:      transferID,
		SourceAccountID: req.SourceAccountID,
		TargetAccountID: req.TargetAccountID,
		Amount:          req.Amount,
		Currency:        req.Currency,
	})

	transferInitiated := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		Version:     1,
		EventType:   domain.EventTransferInitiated,
		Payload:     transferPayload,
		CreatedAt:   now,
	}

	// ACCOUNT_DEBITED event on the source account.
	srcVersion, err := s.eventStore.GetCurrentVersion(ctx, req.SourceAccountID)
	if err != nil {
		return nil, err
	}
	debitPayload, _ := json.Marshal(domain.AccountDebitedPayload{
		AccountID: req.SourceAccountID,
		Amount:    req.Amount,
		Currency:  req.Currency,
		Reference: "TRANSFER:" + transferID,
	})
	debitEvent := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: req.SourceAccountID,
		Version:     srcVersion + 1,
		EventType:   domain.EventAccountDebited,
		Payload:     debitPayload,
		CreatedAt:   now,
	}

	src.Balance -= req.Amount
	src.Version = debitEvent.Version
	src.UpdatedAt = now

	outboxTransfer := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		EventType:   string(domain.EventTransferInitiated),
		Payload:     transferPayload,
		CreatedAt:   now,
	}
	outboxDebit := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: req.SourceAccountID,
		EventType:   string(domain.EventAccountDebited),
		Payload:     debitPayload,
		CreatedAt:   now,
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(transferInitiated).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(debitEvent).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model(src).WherePK().Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(outboxTransfer).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(outboxDebit).Exec(ctx); err != nil {
			return err
		}
		if idempotencyKey != "" {
			evtBytes, _ := json.Marshal(transferInitiated)
			rec := &domain.IdempotencyRecord{
				Key:        idempotencyKey,
				StatusCode: 202,
				Response:   evtBytes,
				CreatedAt:  now,
			}
			if _, err := tx.NewInsert().Model(rec).On("CONFLICT (key) DO NOTHING").Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initiate transfer: transaction: %w", err)
	}

	// Best-effort Kafka publish.
	_ = s.kafkaProducer.PublishEvent(ctx, transferInitiated)
	_ = s.kafkaProducer.PublishEvent(ctx, debitEvent)

	return transferInitiated, nil
}
