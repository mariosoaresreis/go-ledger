package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go-ledger/internal/services"
)

// TransferHandler holds the transfer command service.
type TransferHandler struct {
	svc services.TransferCommandService
}

// NewTransferHandler creates a TransferHandler.
func NewTransferHandler(svc services.TransferCommandService) *TransferHandler {
	return &TransferHandler{svc: svc}
}

// InitiateTransfer godoc
//
//	@Summary		Initiate a transfer
//	@Description	Initiates a choreography saga: debits source, credits target
//	@Tags			transfers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string						false	"Idempotency key (UUID)"
//	@Param			body			body		services.TransferRequest	true	"Transfer request"
//	@Success		202				{object}	domain.LedgerEvent
//	@Failure		400				{object}	ErrorResponse
//	@Failure		422				{object}	ErrorResponse
//	@Failure		500				{object}	ErrorResponse
//	@Router			/v1/transfers [post]
func (h *TransferHandler) InitiateTransfer(c *gin.Context) {
	idempotencyKey := c.GetHeader("Idempotency-Key")

	var req services.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	event, err := h.svc.InitiateTransfer(c.Request.Context(), req, idempotencyKey)
	if err != nil {
		writeError(c.Writer, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(c.Writer, http.StatusAccepted, event)
}
