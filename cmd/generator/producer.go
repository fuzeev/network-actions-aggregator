package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"network-actions-aggregator/internal/domain/entity"
)

// KafkaProducer отправляет события в Kafka
type KafkaProducer struct {
	writer *kafka.Writer
}

// NewKafkaProducer создает новый Kafka producer
func NewKafkaProducer(brokers []string, topic string) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{}, // Партиционирование по ключу (user_id)
		BatchSize:    100,           // Батчинг для производительности
		BatchTimeout: 10 * time.Millisecond,
		Compression:  kafka.Snappy,
		RequiredAcks: kafka.RequireOne, // Ждём подтверждения от лидера
		Async:        false,            // Синхронная отправка для точности метрик
	}

	return &KafkaProducer{
		writer: writer,
	}
}

// eventMessage структура сообщения для Kafka (JSON)
type eventMessage struct {
	EventID     string                 `json:"event_id"`
	SourceID    string                 `json:"source_id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"`
	StartedAt   string                 `json:"started_at"`
	EndedAt     *string                `json:"ended_at,omitempty"`
	DurationSec *int                   `json:"duration_sec,omitempty"`
	BytesUp     int64                  `json:"bytes_up,omitempty"`
	BytesDown   int64                  `json:"bytes_down,omitempty"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
}

// SendEvent отправляет одно событие в Kafka
func (p *KafkaProducer) SendEvent(ctx context.Context, event *entity.Event) error {
	msg := p.eventToMessage(event)

	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(event.UserID.String()), // Партиционирование по user_id
		Value: value,
		Time:  time.Now(),
	}

	return p.writer.WriteMessages(ctx, kafkaMsg)
}

// SendBatch отправляет пакет событий в Kafka
func (p *KafkaProducer) SendBatch(ctx context.Context, events []*entity.Event) error {
	messages := make([]kafka.Message, len(events))

	for i, event := range events {
		msg := p.eventToMessage(event)
		value, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal event %s: %w", event.EventID, err)
		}

		messages[i] = kafka.Message{
			Key:   []byte(event.UserID.String()),
			Value: value,
			Time:  time.Now(),
		}
	}

	return p.writer.WriteMessages(ctx, messages...)
}

// eventToMessage конвертирует entity.Event в eventMessage для JSON
func (p *KafkaProducer) eventToMessage(event *entity.Event) eventMessage {
	msg := eventMessage{
		EventID:     event.EventID.String(),
		SourceID:    event.SourceID.String(),
		UserID:      event.UserID.String(),
		Type:        string(event.Type),
		StartedAt:   event.StartedAt.Format(time.RFC3339),
		DurationSec: event.DurationSec,
		BytesUp:     event.BytesUp,
		BytesDown:   event.BytesDown,
		Meta:        event.Meta,
	}

	if event.EndedAt != nil {
		endedAt := event.EndedAt.Format(time.RFC3339)
		msg.EndedAt = &endedAt
	}

	return msg
}

// Close закрывает producer
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

// Stats возвращает статистику producer'а
func (p *KafkaProducer) Stats() kafka.WriterStats {
	return p.writer.Stats()
}
