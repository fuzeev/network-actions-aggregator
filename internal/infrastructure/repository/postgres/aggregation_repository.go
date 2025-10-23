package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/domain/repository"
	"network-actions-aggregator/internal/infrastructure/repository/postgres/gorm_models"
)

// aggregationRepository - реализация репозитория агрегаций для PostgreSQL
type aggregationRepository struct {
	db *gorm.DB
}

// NewAggregationRepository создает новый экземпляр репозитория агрегаций
func NewAggregationRepository(db *gorm.DB) repository.AggregationRepository {
	return &aggregationRepository{db: db}
}

// UpsertDayAggregation обновляет или вставляет агрегацию за день
func (r *aggregationRepository) UpsertDayAggregation(ctx context.Context, day time.Time, eventType entity.EventType,
	countEvents int64, sumDuration int64, sumBytesUp int64, sumBytesDown int64) error {

	model := &gorm_models.DayAggregation{
		Day:          day.Truncate(24 * time.Hour),
		Type:         eventType.String(),
		CountEvents:  countEvents,
		SumDuration:  sumDuration,
		SumBytesUp:   sumBytesUp,
		SumBytesDown: sumBytesDown,
		UpdatedAt:    time.Now(),
	}

	// Используем Clauses для ON CONFLICT с инкрементальным обновлением
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "day"}, {Name: "type"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"count_events":   gorm.Expr("agg_day.count_events + ?", countEvents),
				"sum_duration":   gorm.Expr("agg_day.sum_duration + ?", sumDuration),
				"sum_bytes_up":   gorm.Expr("agg_day.sum_bytes_up + ?", sumBytesUp),
				"sum_bytes_down": gorm.Expr("agg_day.sum_bytes_down + ?", sumBytesDown),
				"updated_at":     gorm.Expr("NOW()"),
			}),
		}).
		Create(model)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert day aggregation: %w", result.Error)
	}

	return nil
}

// GetDayAggregations возвращает агрегации по дням за период
func (r *aggregationRepository) GetDayAggregations(ctx context.Context, from, to time.Time) ([]*entity.DayAggregation, error) {
	var models []*gorm_models.DayAggregation

	result := r.db.WithContext(ctx).
		Where("day >= ? AND day <= ?", from.Truncate(24*time.Hour), to.Truncate(24*time.Hour)).
		Order("day, type").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get day aggregations: %w", result.Error)
	}

	return gorm_models.ToDomainDayAggregationBatch(models), nil
}

// GetDayAggregationsByType возвращает агрегации по дням для типа
func (r *aggregationRepository) GetDayAggregationsByType(ctx context.Context, eventType entity.EventType, from, to time.Time) ([]*entity.DayAggregation, error) {
	var models []*gorm_models.DayAggregation

	result := r.db.WithContext(ctx).
		Where("day >= ? AND day <= ? AND type = ?",
			from.Truncate(24*time.Hour),
			to.Truncate(24*time.Hour),
			eventType.String()).
		Order("day").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get day aggregations by type: %w", result.Error)
	}

	return gorm_models.ToDomainDayAggregationBatch(models), nil
}

// UpsertMonthAggregation обновляет или вставляет агрегацию за месяц
func (r *aggregationRepository) UpsertMonthAggregation(ctx context.Context, month time.Time, eventType entity.EventType,
	countEvents int64, sumDuration int64, sumBytesUp int64, sumBytesDown int64) error {

	// Приводим к первому числу месяца
	firstDayOfMonth := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())

	model := &gorm_models.MonthAggregation{
		Month:        firstDayOfMonth,
		Type:         eventType.String(),
		CountEvents:  countEvents,
		SumDuration:  sumDuration,
		SumBytesUp:   sumBytesUp,
		SumBytesDown: sumBytesDown,
		UpdatedAt:    time.Now(),
	}

	// Используем Clauses для ON CONFLICT с инкрементальным обновлением
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "month"}, {Name: "type"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"count_events":   gorm.Expr("agg_month.count_events + ?", countEvents),
				"sum_duration":   gorm.Expr("agg_month.sum_duration + ?", sumDuration),
				"sum_bytes_up":   gorm.Expr("agg_month.sum_bytes_up + ?", sumBytesUp),
				"sum_bytes_down": gorm.Expr("agg_month.sum_bytes_down + ?", sumBytesDown),
				"updated_at":     gorm.Expr("NOW()"),
			}),
		}).
		Create(model)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert month aggregation: %w", result.Error)
	}

	return nil
}

// GetMonthAggregations возвращает агрегации по месяцам за период
func (r *aggregationRepository) GetMonthAggregations(ctx context.Context, from, to time.Time) ([]*entity.MonthAggregation, error) {
	// Приводим к первому числу месяца
	fromMonth := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
	toMonth := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, to.Location())

	var models []*gorm_models.MonthAggregation

	result := r.db.WithContext(ctx).
		Where("month >= ? AND month <= ?", fromMonth, toMonth).
		Order("month, type").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get month aggregations: %w", result.Error)
	}

	return gorm_models.ToDomainMonthAggregationBatch(models), nil
}

// GetMonthAggregationsByType возвращает агрегации по месяцам для типа
func (r *aggregationRepository) GetMonthAggregationsByType(ctx context.Context, eventType entity.EventType, from, to time.Time) ([]*entity.MonthAggregation, error) {
	// Приводим к первому числу месяца
	fromMonth := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
	toMonth := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, to.Location())

	var models []*gorm_models.MonthAggregation

	result := r.db.WithContext(ctx).
		Where("month >= ? AND month <= ? AND type = ?", fromMonth, toMonth, eventType.String()).
		Order("month").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get month aggregations by type: %w", result.Error)
	}

	return gorm_models.ToDomainMonthAggregationBatch(models), nil
}
