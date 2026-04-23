package domain

import "time"

// Account is the write-side aggregate root. It is reconstructed from events.
type Account struct {
	ID        string        `bun:"id,pk"`
	OwnerID   string        `bun:"owner_id,notnull"`
	Currency  string        `bun:"currency,notnull"`
	Balance   int64         `bun:"balance,notnull,default:0"` // stored as minor units (cents)
	Status    AccountStatus `bun:"status,notnull,default:'ACTIVE'"`
	Version   int64         `bun:"version,notnull,default:0"`
	CreatedAt time.Time     `bun:"created_at,notnull,default:now()"`
	UpdatedAt time.Time     `bun:"updated_at,notnull,default:now()"`
}

func (Account) TableName() string { return "accounts" }

// AccountCreatedPayload is the JSON payload for EventAccountCreated.
type AccountCreatedPayload struct {
	AccountID string `json:"accountId"`
	OwnerID   string `json:"ownerId"`
	Currency  string `json:"currency"`
}

// AccountCreditedPayload is the JSON payload for EventAccountCredited.
type AccountCreditedPayload struct {
	AccountID string `json:"accountId"`
	Amount    int64  `json:"amount"` // minor units
	Currency  string `json:"currency"`
	Reference string `json:"reference"`
}

// AccountDebitedPayload is the JSON payload for EventAccountDebited.
type AccountDebitedPayload struct {
	AccountID string `json:"accountId"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Reference string `json:"reference"`
}

// AccountStatusChangedPayload is the JSON payload for EventAccountStatusChanged.
type AccountStatusChangedPayload struct {
	AccountID string        `json:"accountId"`
	OldStatus AccountStatus `json:"oldStatus"`
	NewStatus AccountStatus `json:"newStatus"`
}

// TransferInitiatedPayload is the JSON payload for EventTransferInitiated.
type TransferInitiatedPayload struct {
	TransferID      string `json:"transferId"`
	SourceAccountID string `json:"sourceAccountId"`
	TargetAccountID string `json:"targetAccountId"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
}
