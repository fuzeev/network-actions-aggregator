package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"network-actions-aggregator/internal/domain/entity"
)

// EventRepository определяет интерфейс для работы с событиями в хранилище
type EventRepository interface {
	// BatchInsert выполняет пакетную вставку событий в БД
	// Использует ON CONFLICT DO NOTHING для обеспечения идемпотентности
	BatchInsert(ctx context.Context, events []*entity.Event) error

	// GetUserEvents возвращает события пользователя за указанный период
	// Поддерживает пагинацию через cursor
	GetUserEvents(ctx context.Context, userID uuid.UUID, from, to time.Time, limit int, cursor string) ([]*entity.Event, string, error)

	// GetEventsByType возвращает события определенного типа за период
	GetEventsByType(ctx context.Context, eventType entity.EventType, from, to time.Time, limit int) ([]*entity.Event, error)

	// GetEventByID возвращает событие по его ID
	GetEventByID(ctx context.Context, eventID uuid.UUID, startedAt time.Time) (*entity.Event, error)

	// CountUserEvents возвращает количество событий пользователя за период
	CountUserEvents(ctx context.Context, userID uuid.UUID, from, to time.Time) (int64, error)

	// GetByEventIDs возвращает события по списку event_id
	GetByEventIDs(ctx context.Context, eventIDs []uuid.UUID) ([]*entity.Event, error)
}
