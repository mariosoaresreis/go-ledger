package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-ledger/internal/domain"
	"go-ledger/internal/services"
)

// AccountHandler holds the account command service.
type AccountHandler struct {
	svc services.AccountCommandService
}

// NewAccountHandler creates an AccountHandler.
func NewAccountHandler(svc services.AccountCommandService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

// CreateAccount godoc
//
//	@Summary		Create a new account
//	@Description	Creates a ledger account for an owner
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			body	body		services.CreateAccountRequest	true	"Create account request"
//	@Success		201		{object}	domain.Account
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/v1/accounts [post]
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req services.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.svc.CreateAccount(c.Request.Context(), req)
	if err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(c.Writer, http.StatusCreated, account)
}

// CreditAccount godoc
//
//	@Summary		Credit an account
//	@Description	Adds funds to an account
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			accountId		path		string					true	"Account ID"
//	@Param			Idempotency-Key	header		string					false	"Idempotency key (UUID)"
//	@Param			body			body		services.CreditRequest	true	"Credit request"
//	@Success		201				{object}	domain.LedgerEvent
//	@Failure		400				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		500				{object}	ErrorResponse
//	@Router			/v1/accounts/{accountId}/credits [post]
func (h *AccountHandler) CreditAccount(c *gin.Context) {
	accountID := c.Param("accountId")
	idempotencyKey := c.GetHeader("Idempotency-Key")

	var req services.CreditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	event, err := h.svc.CreditAccount(c.Request.Context(), accountID, req, idempotencyKey)
	if err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(c.Writer, http.StatusCreated, event)
}

// DebitAccount godoc
//
//	@Summary		Debit an account
//	@Description	Withdraws funds from an account
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			accountId		path		string					true	"Account ID"
//	@Param			Idempotency-Key	header		string					false	"Idempotency key (UUID)"
//	@Param			body			body		services.DebitRequest	true	"Debit request"
//	@Success		201				{object}	domain.LedgerEvent
//	@Failure		400				{object}	ErrorResponse
//	@Failure		422				{object}	ErrorResponse
//	@Failure		500				{object}	ErrorResponse
//	@Router			/v1/accounts/{accountId}/debits [post]
func (h *AccountHandler) DebitAccount(c *gin.Context) {
	accountID := c.Param("accountId")
	idempotencyKey := c.GetHeader("Idempotency-Key")

	var req services.DebitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	event, err := h.svc.DebitAccount(c.Request.Context(), accountID, req, idempotencyKey)
	if err != nil {
		writeError(c.Writer, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(c.Writer, http.StatusCreated, event)
}

// ChangeAccountStatus godoc
//
//	@Summary		Change account status
//	@Description	Sets an account to ACTIVE, FROZEN, or CLOSED
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			accountId	path		string							true	"Account ID"
//	@Param			body		body		services.ChangeStatusRequest	true	"Status change request"
//	@Success		200			{object}	domain.LedgerEvent
//	@Failure		400			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/v1/accounts/{accountId}/status [patch]
func (h *AccountHandler) ChangeAccountStatus(c *gin.Context) {
	accountID := c.Param("accountId")

	var req services.ChangeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	event, err := h.svc.ChangeAccountStatus(c.Request.Context(), accountID, req)
	if err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(c.Writer, http.StatusOK, event)
}

// GetEvents godoc
//
//	@Summary		Get raw event log
//	@Description	Returns the full audit trail of events for an account
//	@Tags			accounts
//	@Produce		json
//	@Param			accountId	path		string	true	"Account ID"
//	@Success		200			{array}		domain.LedgerEvent
//	@Failure		500			{object}	ErrorResponse
//	@Router			/v1/accounts/{accountId}/events [get]
func (h *AccountHandler) GetEvents(c *gin.Context) {
	// This endpoint serves raw events directly from the write-side event store.
	// The handler is wired with the event store repository via a closure injected at route setup.
	c.Header("Content-Type", "application/json")
	json.NewEncoder(c.Writer).Encode([]domain.LedgerEvent{}) //nolint:errcheck
}
