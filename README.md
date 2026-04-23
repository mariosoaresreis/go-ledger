# go-ledger (Command Service)

Write side of the CQRS ledger. This service validates and processes mutating commands, persists immutable events in PostgreSQL, and writes transactional outbox records for Kafka relay.

## Features

- `POST /api/v1/accounts`
- `POST /api/v1/accounts/{accountId}/credits`
- `POST /api/v1/accounts/{accountId}/debits`
- `POST /api/v1/transfers`
  - Emits `TRANSFER_INITIATED`, then attempts `ACCOUNT_CREDITED` + `TRANSFER_COMPLETED`
  - On credit failure, emits compensating `ACCOUNT_CREDITED` (source refund) + `TRANSFER_REVERSED`
- `PATCH /api/v1/accounts/{accountId}/status`
- `GET /api/v1/accounts/{accountId}/events` (raw audit log from event store)
- Swagger UI at `GET /swagger/index.html`

## Project layout

- `config/` runtime config loading
- `internal/api/` HTTP handlers and routing
- `internal/services/` command and saga logic
- `internal/repository/` PostgreSQL repositories
- `internal/domain/` aggregate and event models
- `internal/kafka/` producer
- `internal/migrations/` SQL schema for write side
- `terraform/` GCP Terraform copied/adapted from `/home/bat/projects/java/ledger/terraform`

## Environment variables

- `PORT` (default `8080`)
- `ENVIRONMENT` (default `local`)
- `LEDGER_DB_HOST`, `LEDGER_DB_PORT`, `LEDGER_DB_USERNAME`, `LEDGER_DB_PASSWORD`, `LEDGER_DB_NAME`, `LEDGER_DB_SSL_MODE`
- `LEDGER_KAFKA_BOOTSTRAP_SERVERS` (default `localhost:9092`)
- `LEDGER_KAFKA_TOPIC` (default `ledger.events`)

## Run locally

```bash
cd /home/bat/projects/go/go-ledger
go mod tidy
go test ./...
go run .
```

## Swagger docs

A minimal docs package is committed in `docs/` so the service builds even before generation.

To regenerate OpenAPI docs from annotations:

```bash
cd /home/bat/projects/go/go-ledger
go install github.com/swaggo/swag/cmd/swag@latest
go generate ./...
```

## Quick API smoke test

```bash
curl -s http://localhost:8080/api/ping
curl -s http://localhost:8080/swagger/index.html | head -n 5
```

## Full local stack (Postgres + Kafka + both services)

Use the compose file at `docker-compose.yml` from this repository root:

```bash
cd /home/bat/projects/go/go-ledger
docker compose up --build -d
```

Optional checks:

```bash
docker compose ps
curl -s http://localhost:8080/api/ping
curl -s http://localhost:8081/api/ping
```

## Terraform (GCP)

Command-service Terraform environment is at:

- `terraform/environments/dev`

Example:

```bash
cd /home/bat/projects/go/go-ledger/terraform/environments/dev
cp terraform.tfvars.example terraform.tfvars
terraform init
terraform plan
```
