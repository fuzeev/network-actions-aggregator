package entity

import (
	"github.com/google/uuid"
	"time"
)

// UserEventDetail представляет детальную информацию о событии пользователя
type UserEventDetail struct {
	// Type - тип события
	Type EventType `json:"type"`

	// StartedAt - время начала события
	StartedAt time.Time `json:"started_at"`

	// EndedAt - время окончания события
	EndedAt *time.Time `json:"ended_at,omitempty"`

	// DurationSec - длительность в секундах
	DurationSec *int `json:"duration_sec,omitempty"`

	// BytesUp - отправленные байты
	BytesUp int64 `json:"bytes_up,omitempty"`

	// BytesDown - полученные байты
	BytesDown int64 `json:"bytes_down,omitempty"`

	// Meta - дополнительные данные
	Meta map[string]interface{} `json:"meta,omitempty"`
}

// UserDetail представляет детализацию по пользователю
type UserDetail struct {
	// UserID - идентификатор пользователя
	UserID uuid.UUID `json:"user_id"`

	// From - начало периода
	From time.Time `json:"from"`

	// To - конец периода
	To time.Time `json:"to"`

	// Items - список событий
	Items []UserEventDetail `json:"items"`

	// NextCursor - курсор для пагинации (опционально)
	NextCursor *string `json:"next_cursor,omitempty"`
}

// Cursor представляет курсор для пагинации
type Cursor struct {
	// LastStartedAt - время последнего события
	LastStartedAt time.Time

	// LastEventID - ID последнего события (для уникальности)
	LastEventID uuid.UUID
}
