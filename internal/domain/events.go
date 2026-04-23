package domain

import "time"

// EventType represents the type of a ledger domain event.
type EventType string

const (
	EventAccountCreated       EventType = "ACCOUNT_CREATED"
	EventAccountCredited      EventType = "ACCOUNT_CREDITED"
	EventAccountDebited       EventType = "ACCOUNT_DEBITED"
	EventAccountStatusChanged EventType = "ACCOUNT_STATUS_CHANGED"
	EventTransferInitiated    EventType = "TRANSFER_INITIATED"
	EventTransferCompleted    EventType = "TRANSFER_COMPLETED"
	EventTransferReversed     EventType = "TRANSFER_REVERSED"
)

// AccountStatus represents the lifecycle state of an account.
type AccountStatus string

const (
	StatusActive AccountStatus = "ACTIVE"
	StatusFrozen AccountStatus = "FROZEN"
	StatusClosed AccountStatus = "CLOSED"
)

// TransactionDirection for filtering purposes.
type TransactionDirection string

const (
	DirectionCredit TransactionDirection = "CREDIT"
	DirectionDebit  TransactionDirection = "DEBIT"
)

// LedgerEvent is the persisted domain event stored in the event store.
type LedgerEvent struct {
	ID          string    `bun:"id,pk"`
	AggregateID string    `bun:"aggregate_id,notnull"`
	Version     int64     `bun:"version,notnull"`
	EventType   EventType `bun:"event_type,notnull"`
	Payload     []byte    `bun:"payload,type:jsonb,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:now()"`
}

func (LedgerEvent) TableName() string { return "ledger_events" }

// OutboxEntry is the transactional outbox record that Debezium/CDC will relay to Kafka.
type OutboxEntry struct {
	ID          string    `bun:"id,pk"`
	AggregateID string    `bun:"aggregate_id,notnull"`
	EventType   string    `bun:"event_type,notnull"`
	Payload     []byte    `bun:"payload,type:jsonb,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:now()"`
	Processed   bool      `bun:"processed,notnull,default:false"`
}

func (OutboxEntry) TableName() string { return "outbox" }

// IdempotencyRecord stores the result of a previously processed command.
type IdempotencyRecord struct {
	Key        string    `bun:"key,pk"`
	StatusCode int       `bun:"status_code,notnull"`
	Response   []byte    `bun:"response,type:jsonb"`
	CreatedAt  time.Time `bun:"created_at,notnull,default:now()"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_keys" }
