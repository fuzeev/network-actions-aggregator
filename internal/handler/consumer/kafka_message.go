package consumer

import (
	"encoding/json"
	"github.com/google/uuid"
	"network-actions-aggregator/internal/domain/entity"
	"time"
)

// KafkaEventMessage представляет сообщение события в Kafka/Redpanda
type KafkaEventMessage struct {
	EventID   string                 `json:"event_id"`
	SourceID  string                 `json:"source_id"`
	UserID    string                 `json:"user_id"`
	Phone     string                 `json:"phone,omitempty"`
	Type      string                 `json:"type"`
	StartedAt string                 `json:"started_at"`
	EndedAt   *string                `json:"ended_at,omitempty"`
	Duration  *int                   `json:"duration_sec,omitempty"`
	BytesUp   int64                  `json:"bytes_up,omitempty"`
	BytesDown int64                  `json:"bytes_down,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// ToEvent конвертирует Kafka сообщение в доменную сущность Event
func (m *KafkaEventMessage) ToEvent() (*entity.Event, error) {
	eventID, err := uuid.Parse(m.EventID)
	if err != nil {
		return nil, err
	}

	sourceID, err := uuid.Parse(m.SourceID)
	if err != nil {
		return nil, err
	}

	userID, err := uuid.Parse(m.UserID)
	if err != nil {
		return nil, err
	}

	startedAt, err := time.Parse(time.RFC3339, m.StartedAt)
	if err != nil {
		return nil, err
	}

	event := &entity.Event{
		EventID:   eventID,
		SourceID:  sourceID,
		UserID:    userID,
		Phone:     m.Phone,
		Type:      entity.EventType(m.Type),
		StartedAt: startedAt,
		BytesUp:   m.BytesUp,
		BytesDown: m.BytesDown,
		Meta:      m.Meta,
		CreatedAt: time.Now(),
	}

	if m.EndedAt != nil {
		endedAt, err := time.Parse(time.RFC3339, *m.EndedAt)
		if err != nil {
			return nil, err
		}
		event.EndedAt = &endedAt
	}

	if m.Duration != nil {
		event.DurationSec = m.Duration
	}

	return event, nil
}

// UnmarshalKafkaMessage десериализует JSON в сообщение
func UnmarshalKafkaMessage(data []byte) (*KafkaEventMessage, error) {
	var msg KafkaEventMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
