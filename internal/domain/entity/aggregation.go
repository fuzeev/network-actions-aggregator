package entity

import "time"

// DayAggregation представляет агрегированные данные за день
type DayAggregation struct {
	// Day - дата дня
	Day time.Time

	// Type - тип события
	Type EventType

	// CountEvents - количество событий
	CountEvents int64

	// SumDuration - сумма длительностей в секундах
	SumDuration int64

	// SumBytesUp - сумма отправленных байт
	SumBytesUp int64

	// SumBytesDown - сумма полученных байт
	SumBytesDown int64

	// UpdatedAt - время последнего обновления
	UpdatedAt time.Time
}

// MonthAggregation представляет агрегированные данные за месяц
type MonthAggregation struct {
	// Month - первое число месяца
	Month time.Time

	// Type - тип события
	Type EventType

	// CountEvents - количество событий
	CountEvents int64

	// SumDuration - сумма длительностей в секундах
	SumDuration int64

	// SumBytesUp - сумма отправленных байт
	SumBytesUp int64

	// SumBytesDown - сумма полученных байт
	SumBytesDown int64

	// UpdatedAt - время последнего обновления
	UpdatedAt time.Time
}

// Granularity представляет гранулярность агрегации
type Granularity string

const (
	// GranularityDay - агрегация по дням
	GranularityDay Granularity = "DAY"

	// GranularityMonth - агрегация по месяцам
	GranularityMonth Granularity = "MONTH"
)

// IsValid проверяет валидность гранулярности
func (g Granularity) IsValid() bool {
	switch g {
	case GranularityDay, GranularityMonth:
		return true
	default:
		return false
	}
}

// AggregationData представляет одну запись агрегированных данных для API
type AggregationData struct {
	// Date - дата (день или месяц)
	Date time.Time

	// Type - тип события
	Type EventType

	// CountEvents - количество событий
	CountEvents int64

	// SumDuration - сумма длительностей в секундах
	SumDuration int64

	// SumBytesUp - сумма отправленных байт
	SumBytesUp int64

	// SumBytesDown - сумма полученных байт
	SumBytesDown int64
}

// AggregationResult представляет результат запроса агрегированных данных
type AggregationResult struct {
	// Granularity - гранулярность (день/месяц)
	Granularity Granularity

	// From - начало периода
	From time.Time

	// To - конец периода
	To time.Time

	// Data - агрегированные данные
	Data []AggregationData
}
