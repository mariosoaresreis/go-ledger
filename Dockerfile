FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ledger-command-service ./main.go

FROM alpine:3.20
RUN adduser -D -H appuser
USER appuser
WORKDIR /app
COPY --from=builder /out/ledger-command-service /app/ledger-command-service
EXPOSE 8080
ENTRYPOINT ["/app/ledger-command-service"]

