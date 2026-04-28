// @title           Ledger Command Service API
// @version         1.0
// @description     Write-side of the CQRS ledger: account creation, credits, debits, transfers.
// @termsOfService  http://example.com/terms/

// @contact.name   Ledger Team
// @contact.email  ledger@example.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

//go:generate swag init -g main.go -o docs
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"

	"go-ledger/config"
	_ "go-ledger/docs"
	"go-ledger/internal/api"
	kafkapkg "go-ledger/internal/kafka"
	"go-ledger/internal/repository"
	"go-ledger/internal/services"
)

func main() {
	cfg := config.Load()

	logrus.Infof("Starting %s (env=%s port=%s)", config.ServiceName, cfg.Environment, cfg.Port)

	// ── Database ──────────────────────────────────────────────────────────────
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBSSLMode,
	)

	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		logrus.Fatalf("cannot initialize database client: %v", err)
	}
	db.SetMaxOpenConns(cfg.DBConns)

	if err := db.PingContext(context.Background()); err != nil {
		logrus.Fatalf("cannot connect to database: %v", err)
	}
	logrus.Info("database: connected")

	// ── Repositories ──────────────────────────────────────────────────────────
	eventStoreRepo := repository.NewEventStoreRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)

	// ── Kafka ─────────────────────────────────────────────────────────────────
	kafkaProducer := kafkapkg.NewProducer(cfg.KafkaBootstrapServers, cfg.KafkaTopic)
	defer kafkaProducer.Close() //nolint:errcheck

	// ── Services ──────────────────────────────────────────────────────────────
	accountSvc := services.NewAccountCommandService(
		db, eventStoreRepo, accountRepo, outboxRepo, idempotencyRepo, kafkaProducer,
	)
	transferSvc := services.NewTransferCommandService(
		db, eventStoreRepo, accountRepo, outboxRepo, idempotencyRepo, kafkaProducer,
	)

	// ── API ───────────────────────────────────────────────────────────────────
	apiServer := api.NewAPI(cfg, accountSvc, transferSvc, eventStoreRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := apiServer.Run(ctx)

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logrus.Infof("received signal %s – shutting down", sig)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.GracefulStopTimeout)
		defer shutdownCancel()
		if err := apiServer.GracefulStop(shutdownCtx); err != nil {
			logrus.Errorf("graceful stop error: %v", err)
		}
	case err := <-errCh:
		if err != nil {
			logrus.Fatalf("server error: %v", err)
		}
	}

	logrus.Info("shutdown complete")
}
