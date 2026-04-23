package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go-ledger/config"
	"go-ledger/internal/repository"
	"go-ledger/internal/services"
)

// API is the main HTTP server struct for the command service.
type API struct {
	cfg            *config.Config
	router         *gin.Engine
	server         *http.Server
	accountSvc     services.AccountCommandService
	transferSvc    services.TransferCommandService
	eventStoreRepo repository.EventStoreRepository
}

// NewAPI creates and configures the API server.
func NewAPI(
	cfg *config.Config,
	accountSvc services.AccountCommandService,
	transferSvc services.TransferCommandService,
	eventStoreRepo repository.EventStoreRepository,
) *API {
	a := &API{
		cfg:            cfg,
		accountSvc:     accountSvc,
		transferSvc:    transferSvc,
		eventStoreRepo: eventStoreRepo,
	}
	a.initialize()
	return a
}

// Title satisfies the Module interface.
func (a *API) Title() string { return "HTTP REST API (command)" }

// GracefulStop shuts down the HTTP server.
func (a *API) GracefulStop(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

// Run starts the server and returns a channel that receives any fatal error.
func (a *API) Run(_ context.Context) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf("%s:%s", a.cfg.Host, a.cfg.Port)
		logrus.Infof("command service: listening on %s", addr)
		a.server = &http.Server{Addr: addr, Handler: a.router}
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
	return errCh
}

func (a *API) initialize() {
	if a.cfg.Environment == "production" || a.cfg.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type", "Idempotency-Key"},
	}))

	// Swagger
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	base := router.Group("/api")
	base.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	base.GET("/livez", func(c *gin.Context) { c.Status(http.StatusOK) })
	base.GET("/readyz", func(c *gin.Context) { c.Status(http.StatusOK) })
	base.GET("/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": config.ServiceName, "version": "1.0.0"})
	})

	v1 := base.Group("/v1")

	// Account commands
	ah := NewAccountHandler(a.accountSvc)
	v1.POST("/accounts", ah.CreateAccount)
	v1.POST("/accounts/:accountId/credits", ah.CreditAccount)
	v1.POST("/accounts/:accountId/debits", ah.DebitAccount)
	v1.PATCH("/accounts/:accountId/status", ah.ChangeAccountStatus)
	v1.GET("/accounts/:accountId/events", func(c *gin.Context) {
		accountID := c.Param("accountId")
		events, err := a.eventStoreRepo.GetEventsByAggregateID(c.Request.Context(), accountID)
		if err != nil {
			writeError(c.Writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(c.Writer, http.StatusOK, events)
	})

	// Transfer commands
	th := NewTransferHandler(a.transferSvc)
	v1.POST("/transfers", th.InitiateTransfer)

	a.router = router
}
