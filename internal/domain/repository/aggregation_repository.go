package repository

import (
	"context"
	"time"

	"network-actions-aggregator/internal/domain/entity"
)

// AggregationRepository определяет интерфейс для работы с агрегациями
type AggregationRepository interface {
	// UpsertDayAggregation обновляет или вставляет агрегацию за день
	UpsertDayAggregation(ctx context.Context, day time.Time, eventType entity.EventType,
		countEvents int64, sumDuration int64, sumBytesUp int64, sumBytesDown int64) error

	// GetDayAggregations возвращает агрегации по дням за период
	GetDayAggregations(ctx context.Context, from, to time.Time) ([]*entity.DayAggregation, error)

	// GetDayAggregationsByType возвращает агрегации по дням для определенного типа событий
	GetDayAggregationsByType(ctx context.Context, eventType entity.EventType, from, to time.Time) ([]*entity.DayAggregation, error)

	// UpsertMonthAggregation обновляет или вставляет агрегацию за месяц
	UpsertMonthAggregation(ctx context.Context, month time.Time, eventType entity.EventType,
		countEvents int64, sumDuration int64, sumBytesUp int64, sumBytesDown int64) error

	// GetMonthAggregations возвращает агрегации по месяцам за период
	GetMonthAggregations(ctx context.Context, from, to time.Time) ([]*entity.MonthAggregation, error)

	// GetMonthAggregationsByType возвращает агрегации по месяцам для определенного типа событий
	GetMonthAggregationsByType(ctx context.Context, eventType entity.EventType, from, to time.Time) ([]*entity.MonthAggregation, error)
}
