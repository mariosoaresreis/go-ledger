package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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
	db            *sqlx.DB
	eventStore    repository.EventStoreRepository
	accountRepo   repository.AccountRepository
	outboxRepo    repository.OutboxRepository
	idempotency   repository.IdempotencyRepository
	kafkaProducer *kafka.Producer
}

// NewTransferCommandService creates a new TransferCommandService.
func NewTransferCommandService(
	db *sqlx.DB,
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
	if tgt.Currency != req.Currency {
		return nil, fmt.Errorf("currency mismatch on target account")
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

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, transferInitiated); err != nil {
			return err
		}
		if err := insertLedgerEvent(ctx, tx, debitEvent); err != nil {
			return err
		}
		if err := updateAccount(ctx, tx, src); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxTransfer); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxDebit); err != nil {
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
			if err := insertIdempotency(ctx, tx, rec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initiate transfer: transaction: %w", err)
	}

	// Best-effort Kafka publish of first saga step.
	_ = s.kafkaProducer.PublishEvent(ctx, transferInitiated)
	_ = s.kafkaProducer.PublishEvent(ctx, debitEvent)

	completedEvent, err := s.completeTransfer(ctx, transferID, req, now)
	if err != nil {
		if compErr := s.compensateTransfer(ctx, transferID, req, now, err.Error()); compErr != nil {
			return nil, fmt.Errorf("transfer completion failed: %v; compensation failed: %v", err, compErr)
		}
		return nil, fmt.Errorf("transfer completion failed and was compensated: %w", err)
	}

	if idempotencyKey != "" {
		evtBytes, _ := json.Marshal(completedEvent)
		rec := &domain.IdempotencyRecord{
			Key:        idempotencyKey,
			StatusCode: 202,
			Response:   evtBytes,
			CreatedAt:  time.Now().UTC(),
		}
		_ = s.idempotency.Save(ctx, rec)
	}

	return transferInitiated, nil
}

func (s *transferCommandService) completeTransfer(ctx context.Context, transferID string, req TransferRequest, startedAt time.Time) (*domain.LedgerEvent, error) {
	tgt, err := s.accountRepo.GetByID(ctx, req.TargetAccountID)
	if err != nil {
		return nil, fmt.Errorf("reload target account: %w", err)
	}
	if tgt.Status != domain.StatusActive {
		return nil, fmt.Errorf("target account became non-active")
	}
	if tgt.Currency != req.Currency {
		return nil, fmt.Errorf("target account currency mismatch")
	}

	tgtVersion, err := s.eventStore.GetCurrentVersion(ctx, req.TargetAccountID)
	if err != nil {
		return nil, err
	}

	creditPayload, _ := json.Marshal(domain.AccountCreditedPayload{
		AccountID: req.TargetAccountID,
		Amount:    req.Amount,
		Currency:  req.Currency,
		Reference: "TRANSFER:" + transferID,
	})
	creditEvent := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: req.TargetAccountID,
		Version:     tgtVersion + 1,
		EventType:   domain.EventAccountCredited,
		Payload:     creditPayload,
		CreatedAt:   time.Now().UTC(),
	}

	completedPayload, _ := json.Marshal(domain.TransferCompletedPayload{
		TransferID:      transferID,
		SourceAccountID: req.SourceAccountID,
		TargetAccountID: req.TargetAccountID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		CompletedAt:     time.Now().UTC().Format(time.RFC3339),
	})
	completedEvent := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		Version:     2,
		EventType:   domain.EventTransferCompleted,
		Payload:     completedPayload,
		CreatedAt:   time.Now().UTC(),
	}

	tgt.Balance += req.Amount
	tgt.Version = creditEvent.Version
	tgt.UpdatedAt = time.Now().UTC()

	outboxCredit := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: req.TargetAccountID,
		EventType:   string(domain.EventAccountCredited),
		Payload:     creditPayload,
		CreatedAt:   startedAt,
	}
	outboxCompleted := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		EventType:   string(domain.EventTransferCompleted),
		Payload:     completedPayload,
		CreatedAt:   startedAt,
	}

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, creditEvent); err != nil {
			return err
		}
		if err := insertLedgerEvent(ctx, tx, completedEvent); err != nil {
			return err
		}
		if err := updateAccount(ctx, tx, tgt); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxCredit); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxCompleted); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.kafkaProducer.PublishEvent(ctx, creditEvent)
	_ = s.kafkaProducer.PublishEvent(ctx, completedEvent)

	return completedEvent, nil
}

func (s *transferCommandService) compensateTransfer(ctx context.Context, transferID string, req TransferRequest, startedAt time.Time, reason string) error {
	src, err := s.accountRepo.GetByID(ctx, req.SourceAccountID)
	if err != nil {
		return fmt.Errorf("reload source account for compensation: %w", err)
	}

	srcVersion, err := s.eventStore.GetCurrentVersion(ctx, req.SourceAccountID)
	if err != nil {
		return err
	}

	creditPayload, _ := json.Marshal(domain.AccountCreditedPayload{
		AccountID: req.SourceAccountID,
		Amount:    req.Amount,
		Currency:  req.Currency,
		Reference: "TRANSFER_REVERSED:" + transferID,
	})
	creditEvent := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: req.SourceAccountID,
		Version:     srcVersion + 1,
		EventType:   domain.EventAccountCredited,
		Payload:     creditPayload,
		CreatedAt:   time.Now().UTC(),
	}

	reversedPayload, _ := json.Marshal(domain.TransferReversedPayload{
		TransferID:      transferID,
		SourceAccountID: req.SourceAccountID,
		TargetAccountID: req.TargetAccountID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Reason:          reason,
	})
	reversedEvent := &domain.LedgerEvent{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		Version:     2,
		EventType:   domain.EventTransferReversed,
		Payload:     reversedPayload,
		CreatedAt:   time.Now().UTC(),
	}

	src.Balance += req.Amount
	src.Version = creditEvent.Version
	src.UpdatedAt = time.Now().UTC()

	outboxCredit := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: req.SourceAccountID,
		EventType:   string(domain.EventAccountCredited),
		Payload:     creditPayload,
		CreatedAt:   startedAt,
	}
	outboxReversed := &domain.OutboxEntry{
		ID:          uuid.NewString(),
		AggregateID: transferID,
		EventType:   string(domain.EventTransferReversed),
		Payload:     reversedPayload,
		CreatedAt:   startedAt,
	}

	err = withTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if err := insertLedgerEvent(ctx, tx, creditEvent); err != nil {
			return err
		}
		if err := insertLedgerEvent(ctx, tx, reversedEvent); err != nil {
			return err
		}
		if err := updateAccount(ctx, tx, src); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxCredit); err != nil {
			return err
		}
		if err := insertOutbox(ctx, tx, outboxReversed); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	_ = s.kafkaProducer.PublishEvent(ctx, creditEvent)
	_ = s.kafkaProducer.PublishEvent(ctx, reversedEvent)

	return nil
}
