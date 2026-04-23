package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
	"go-ledger/internal/domain"
)

// Producer wraps a kafka-go writer.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer creates a new Kafka producer for the given bootstrap servers and topic.
func NewProducer(bootstrapServers, topic string) *Producer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(bootstrapServers),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		MaxAttempts:  3,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	return &Producer{writer: w}
}

// PublishEvent serializes and writes a LedgerEvent to Kafka.
func (p *Producer) PublishEvent(ctx context.Context, event *domain.LedgerEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("kafka producer: marshal event: %w", err)
	}

	msg := kafka.Message{
		Key:   []byte(event.AggregateID),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(event.EventType)},
		},
		Time: event.CreatedAt,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		logrus.WithError(err).WithField("aggregate_id", event.AggregateID).
			Error("kafka: failed to publish event")
		return fmt.Errorf("kafka producer: write: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"aggregate_id": event.AggregateID,
		"event_type":   event.EventType,
		"version":      event.Version,
	}).Info("kafka: event published")

	return nil
}

// Close closes the underlying writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
