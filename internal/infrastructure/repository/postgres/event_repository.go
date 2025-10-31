package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/domain/repository"
	"network-actions-aggregator/internal/infrastructure/repository/postgres/gorm_models"
)

// eventRepository - реализация репозитория событий для PostgreSQL
type eventRepository struct {
	db *gorm.DB
}

// NewEventRepository создает новый экземпляр репозитория событий
func NewEventRepository(db *gorm.DB) repository.EventRepository {
	return &eventRepository{db: db}
}

// BatchInsert выполняет пакетную вставку событий с защитой от дублей
func (r *eventRepository) BatchInsert(ctx context.Context, events []*entity.Event) error {
	if len(events) == 0 {
		return nil
	}

	// Валидация событий
	for i, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("invalid event at index %d: %w", i, err)
		}
	}

	// Конвертируем доменные сущности в GORM модели
	models := gorm_models.FromDomainEventBatch(events)

	// Выполняем пакетную вставку с ON CONFLICT DO NOTHING
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}, {Name: "started_at"}},
			DoNothing: true,
		}).
		CreateInBatches(models, len(models))

	if result.Error != nil {
		return fmt.Errorf("failed to batch insert events: %w", result.Error)
	}

	return nil
}

// GetUserEvents возвращает события пользователя за период с пагинацией
func (r *eventRepository) GetUserEvents(ctx context.Context, userID uuid.UUID, from, to time.Time, limit int, cursor string) ([]*entity.Event, string, error) {
	// TODO: реализовать пагинацию с курсором
	// Курсор можно закодировать как base64(started_at + event_id)
	return nil, "", fmt.Errorf("not implemented yet")
}

// GetEventsByType возвращает события определенного типа за период
func (r *eventRepository) GetEventsByType(ctx context.Context, eventType entity.EventType, from, to time.Time, limit int) ([]*entity.Event, error) {
	var models []*gorm_models.Event

	result := r.db.WithContext(ctx).
		Where("type = ? AND started_at >= ? AND started_at <= ?", eventType.String(), from, to).
		Order("started_at DESC").
		Limit(limit).
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", result.Error)
	}

	return gorm_models.ToDomainEventBatch(models), nil
}

// GetEventByID возвращает событие по ID
func (r *eventRepository) GetEventByID(ctx context.Context, eventID uuid.UUID, startedAt time.Time) (*entity.Event, error) {
	var model gorm_models.Event

	result := r.db.WithContext(ctx).
		Where("event_id = ? AND started_at = ?", eventID, startedAt).
		First(&model)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("event not found")
		}
		return nil, fmt.Errorf("failed to get event by id: %w", result.Error)
	}

	return model.ToDomain(), nil
}

// CountUserEvents возвращает количество событий пользователя за период
func (r *eventRepository) CountUserEvents(ctx context.Context, userID uuid.UUID, from, to time.Time) (int64, error) {
	var count int64

	result := r.db.WithContext(ctx).
		Model(&gorm_models.Event{}).
		Where("user_id = ? AND started_at >= ? AND started_at <= ?", userID, from, to).
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count user events: %w", result.Error)
	}

	return count, nil
}

// GetByEventIDs возвращает события по списку event_id
func (r *eventRepository) GetByEventIDs(ctx context.Context, eventIDs []uuid.UUID) ([]*entity.Event, error) {
	if len(eventIDs) == 0 {
		return []*entity.Event{}, nil
	}

	var models []*gorm_models.Event

	result := r.db.WithContext(ctx).
		Where("event_id IN ?", eventIDs).
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get events by ids: %w", result.Error)
	}

	return gorm_models.ToDomainEventBatch(models), nil
}

// GetByTimeRange возвращает все события за указанный период времени
func (r *eventRepository) GetByTimeRange(ctx context.Context, from, to time.Time) ([]*entity.Event, error) {
	var models []*gorm_models.Event

	result := r.db.WithContext(ctx).
		Where("started_at >= ? AND started_at <= ?", from, to).
		Order("started_at ASC").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get events by time range: %w", result.Error)
	}

	return gorm_models.ToDomainEventBatch(models), nil
}
