package gorm_models

import (
	"time"

	"github.com/google/uuid"

	"network-actions-aggregator/internal/domain/entity"
)

// Event - GORM модель для таблицы events_raw
type Event struct {
	EventID     uuid.UUID  `gorm:"column:event_id;primaryKey"`
	SourceID    uuid.UUID  `gorm:"column:source_id;not null"`
	UserID      uuid.UUID  `gorm:"column:user_id;not null;index:idx_events_raw_user_started"`
	Phone       string     `gorm:"column:phone;not null"`
	Type        string     `gorm:"column:type;not null"`
	StartedAt   time.Time  `gorm:"column:started_at;primaryKey"`
	EndedAt     *time.Time `gorm:"column:ended_at"`
	DurationSec *int       `gorm:"column:duration_sec"`
	BytesUp     int64      `gorm:"column:bytes_up;default:0"`
	BytesDown   int64      `gorm:"column:bytes_down;default:0"`
	Meta        JSONB      `gorm:"column:meta;type:jsonb;default:'{}'"`
	CreatedAt   time.Time  `gorm:"column:created_at;default:now()"`
}

// TableName указывает имя таблицы для GORM
func (Event) TableName() string {
	return "events_raw"
}

// FromDomain конвертирует доменную сущность в GORM модель
func (m *Event) FromDomain(e *entity.Event) {
	m.EventID = e.EventID
	m.SourceID = e.SourceID
	m.UserID = e.UserID
	m.Phone = e.Phone
	m.Type = e.Type.String()
	m.StartedAt = e.StartedAt
	m.EndedAt = e.EndedAt
	m.DurationSec = e.DurationSec
	m.BytesUp = e.BytesUp
	m.BytesDown = e.BytesDown

	// Конвертируем map в JSONB
	if e.Meta != nil {
		m.Meta = JSONB(e.Meta)
	} else {
		m.Meta = make(JSONB)
	}

	m.CreatedAt = e.CreatedAt
}

// ToDomain конвертирует GORM модель в доменную сущность
func (m *Event) ToDomain() *entity.Event {
	return &entity.Event{
		EventID:     m.EventID,
		SourceID:    m.SourceID,
		UserID:      m.UserID,
		Phone:       m.Phone,
		Type:        entity.EventType(m.Type),
		StartedAt:   m.StartedAt,
		EndedAt:     m.EndedAt,
		DurationSec: m.DurationSec,
		BytesUp:     m.BytesUp,
		BytesDown:   m.BytesDown,
		Meta:        map[string]interface{}(m.Meta), // Конвертируем JSONB в map
		CreatedAt:   m.CreatedAt,
	}
}

// FromDomainBatch конвертирует слайс доменных сущностей в GORM модели
func FromDomainEventBatch(events []*entity.Event) []*Event {
	models := make([]*Event, len(events))
	for i, e := range events {
		model := &Event{}
		model.FromDomain(e)
		models[i] = model
	}
	return models
}

// ToDomainBatch конвертирует слайс GORM моделей в доменные сущности
func ToDomainEventBatch(models []*Event) []*entity.Event {
	events := make([]*entity.Event, len(models))
	for i, m := range models {
		events[i] = m.ToDomain()
	}
	return events
}
