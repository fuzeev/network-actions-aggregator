package gorm_models

import (
	"time"

	"network-actions-aggregator/internal/domain/entity"
)

// DayAggregation - GORM модель для таблицы agg_day
type DayAggregation struct {
	Day          time.Time `gorm:"column:day;primaryKey"`
	Type         string    `gorm:"column:type;primaryKey"`
	CountEvents  int64     `gorm:"column:count_events;default:0"`
	SumDuration  int64     `gorm:"column:sum_duration;default:0"`
	SumBytesUp   int64     `gorm:"column:sum_bytes_up;default:0"`
	SumBytesDown int64     `gorm:"column:sum_bytes_down;default:0"`
	UpdatedAt    time.Time `gorm:"column:updated_at;default:now()"`
}

// TableName указывает имя таблицы для GORM
func (DayAggregation) TableName() string {
	return "agg_day"
}

// FromDomain конвертирует доменную сущность в GORM модель
func (m *DayAggregation) FromDomain(d *entity.DayAggregation) {
	m.Day = d.Day
	m.Type = d.Type.String()
	m.CountEvents = d.CountEvents
	m.SumDuration = d.SumDuration
	m.SumBytesUp = d.SumBytesUp
	m.SumBytesDown = d.SumBytesDown
	m.UpdatedAt = d.UpdatedAt
}

// ToDomain конвертирует GORM модель в доменную сущность
func (m *DayAggregation) ToDomain() *entity.DayAggregation {
	return &entity.DayAggregation{
		Day:          m.Day,
		Type:         entity.EventType(m.Type),
		CountEvents:  m.CountEvents,
		SumDuration:  m.SumDuration,
		SumBytesUp:   m.SumBytesUp,
		SumBytesDown: m.SumBytesDown,
		UpdatedAt:    m.UpdatedAt,
	}
}

// ToDomainBatch конвертирует слайс GORM моделей в доменные сущности
func ToDomainDayAggregationBatch(models []*DayAggregation) []*entity.DayAggregation {
	aggregations := make([]*entity.DayAggregation, len(models))
	for i, m := range models {
		aggregations[i] = m.ToDomain()
	}
	return aggregations
}
