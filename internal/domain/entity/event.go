package entity

import (
	"github.com/google/uuid"
	"time"
)

// Event представляет событие в телеком сети
type Event struct {
	// EventID - уникальный идентификатор события
	EventID uuid.UUID

	// SourceID - идентификатор источника события (провайдер)
	SourceID uuid.UUID

	// UserID - идентификатор пользователя
	UserID uuid.UUID

	// Phone - номер телефона пользователя
	Phone string

	// Type - тип события (звонок, SMS, данные)
	Type EventType

	// StartedAt - время начала события
	StartedAt time.Time

	// EndedAt - время окончания события (может быть nil для мгновенных событий)
	EndedAt *time.Time

	// DurationSec - длительность в секундах (для звонков и сессий)
	DurationSec *int

	// BytesUp - количество отправленных байт (для DATA)
	BytesUp int64

	// BytesDown - количество полученных байт (для DATA)
	BytesDown int64

	// Meta - дополнительные метаданные (JSON)
	Meta map[string]interface{}

	// CreatedAt - время создания записи в системе
	CreatedAt time.Time
}

// Validate проверяет валидность события
func (e *Event) Validate() error {
	if e.EventID == uuid.Nil {
		return ErrInvalidEventID
	}

	if e.SourceID == uuid.Nil {
		return ErrInvalidSourceID
	}

	if e.UserID == uuid.Nil {
		return ErrInvalidUserID
	}

	if e.Phone == "" {
		return ErrInvalidPhone
	}

	if !e.Type.IsValid() {
		return ErrInvalidEventType
	}

	if e.StartedAt.IsZero() {
		return ErrInvalidStartedAt
	}

	// Проверка для событий с длительностью
	if e.Type == EventTypeCallOut || e.Type == EventTypeCallIn || e.Type == EventTypeData {
		if e.EndedAt != nil && e.EndedAt.Before(e.StartedAt) {
			return ErrEndedBeforeStarted
		}
	}

	return nil
}
