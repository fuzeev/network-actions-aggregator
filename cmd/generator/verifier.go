package main

import (
	"context"
	"fmt"
	"network-actions-aggregator/internal/domain/entity"
	"sync"
	"time"

	"github.com/google/uuid"
	"network-actions-aggregator/internal/domain/repository"
)

// Verifier проверяет что события попали в базу данных
type Verifier struct {
	eventRepo repository.EventRepository

	mu             sync.RWMutex
	sentEvents     map[uuid.UUID]time.Time // event_id -> время отправки
	verifiedEvents map[uuid.UUID]time.Time // event_id -> время проверки
	missingEvents  map[uuid.UUID]time.Time // event_id -> последняя попытка проверки
}

// NewVerifier создает новый верификатор
func NewVerifier(eventRepo repository.EventRepository) *Verifier {
	return &Verifier{
		eventRepo:      eventRepo,
		sentEvents:     make(map[uuid.UUID]time.Time),
		verifiedEvents: make(map[uuid.UUID]time.Time),
		missingEvents:  make(map[uuid.UUID]time.Time),
	}
}

// TrackSent регистрирует отправленное событие
func (v *Verifier) TrackSent(eventID uuid.UUID) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sentEvents[eventID] = time.Now()
}

// TrackSentBatch регистрирует пакет отправленных событий
func (v *Verifier) TrackSentBatch(eventIDs []uuid.UUID) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	for _, id := range eventIDs {
		v.sentEvents[id] = now
	}
}

// VerifyAll проверяет все отправленные события в базе данных
func (v *Verifier) VerifyAll(ctx context.Context) error {
	v.mu.Lock()
	eventIDsToCheck := make([]uuid.UUID, 0, len(v.sentEvents))
	for id := range v.sentEvents {
		if _, verified := v.verifiedEvents[id]; !verified {
			eventIDsToCheck = append(eventIDsToCheck, id)
		}
	}
	v.mu.Unlock()

	if len(eventIDsToCheck) == 0 {
		return nil
	}

	// Проверяем события батчами по 1000
	batchSize := 1000
	now := time.Now()

	for i := 0; i < len(eventIDsToCheck); i += batchSize {
		end := i + batchSize
		if end > len(eventIDsToCheck) {
			end = len(eventIDsToCheck)
		}
		batch := eventIDsToCheck[i:end]

		// Проверяем существование событий в БД
		events, err := v.eventRepo.GetByEventIDs(ctx, batch)
		if err != nil {
			return fmt.Errorf("failed to verify events: %w", err)
		}

		// Отмечаем найденные события
		v.mu.Lock()
		for _, event := range events {
			v.verifiedEvents[event.EventID] = now
			delete(v.missingEvents, event.EventID)
		}

		// Отмечаем не найденные события
		foundIDs := make(map[uuid.UUID]bool)
		for _, event := range events {
			foundIDs[event.EventID] = true
		}
		for _, id := range batch {
			if !foundIDs[id] {
				v.missingEvents[id] = now
			}
		}
		v.mu.Unlock()
	}

	return nil
}

// Stats возвращает статистику верификации
func (v *Verifier) Stats() VerificationStats {
	v.mu.RLock()
	defer v.mu.RUnlock()

	stats := VerificationStats{
		TotalSent: len(v.sentEvents),
		Verified:  len(v.verifiedEvents),
		Missing:   len(v.missingEvents),
		Pending:   len(v.sentEvents) - len(v.verifiedEvents),
	}

	// Вычисляем средние задержки
	if len(v.verifiedEvents) > 0 {
		var totalLatency time.Duration
		for eventID, verifiedAt := range v.verifiedEvents {
			if sentAt, exists := v.sentEvents[eventID]; exists {
				totalLatency += verifiedAt.Sub(sentAt)
			}
		}
		stats.AvgLatency = totalLatency / time.Duration(len(v.verifiedEvents))
	}

	// Вычисляем процент успеха
	if stats.TotalSent > 0 {
		stats.SuccessRate = float64(stats.Verified) / float64(stats.TotalSent) * 100
	}

	return stats
}

// Reset сбрасывает все данные верификатора
func (v *Verifier) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.sentEvents = make(map[uuid.UUID]time.Time)
	v.verifiedEvents = make(map[uuid.UUID]time.Time)
	v.missingEvents = make(map[uuid.UUID]time.Time)
}

// VerificationStats статистика верификации
type VerificationStats struct {
	TotalSent   int           // Всего отправлено событий
	Verified    int           // Успешно проверено в БД
	Missing     int           // Не найдено в БД
	Pending     int           // Ожидают проверки
	AvgLatency  time.Duration // Средняя задержка от отправки до появления в БД
	SuccessRate float64       // Процент успешно записанных событий
}

func (s VerificationStats) String() string {
	return fmt.Sprintf(
		"Sent: %d | Verified: %d | Missing: %d | Pending: %d | Success: %.2f%% | Avg Latency: %s",
		s.TotalSent,
		s.Verified,
		s.Missing,
		s.Pending,
		s.SuccessRate,
		s.AvgLatency.Round(time.Millisecond),
	)
}

// VerifyAggregations проверяет корректность агрегаций
func (v *Verifier) VerifyAggregations(ctx context.Context, aggregationRepo repository.AggregationRepository, events []*entity.Event) error {
	// Вычисляем ожидаемые агрегаты из исходных событий
	expectedDay, expectedMonth := calculateExpectedAggregations(events)

	// Определяем временной диапазон
	if len(events) == 0 {
		return fmt.Errorf("no events to verify aggregations")
	}

	var minTime, maxTime time.Time
	for i, event := range events {
		if i == 0 || event.StartedAt.Before(minTime) {
			minTime = event.StartedAt
		}
		if i == 0 || event.StartedAt.After(maxTime) {
			maxTime = event.StartedAt
		}
	}

	// Получаем агрегаты из БД
	dayAggs, err := aggregationRepo.GetDayAggregations(ctx, minTime, maxTime)
	if err != nil {
		return fmt.Errorf("failed to get day aggregations: %w", err)
	}

	monthAggs, err := aggregationRepo.GetMonthAggregations(ctx, minTime, maxTime)
	if err != nil {
		return fmt.Errorf("failed to get month aggregations: %w", err)
	}

	// Преобразуем в карты для сравнения
	actualDay := convertDayAggsToMap(dayAggs)
	actualMonth := convertMonthAggsToMap(monthAggs)

	// Сравниваем дневные агрегаты
	dayErrors := compareDayAggregations(expectedDay, actualDay)
	if len(dayErrors) > 0 {
		return fmt.Errorf("day aggregation verification failed: %v", dayErrors)
	}

	// Сравниваем месячные агрегаты
	monthErrors := compareMonthAggregations(expectedMonth, actualMonth)
	if len(monthErrors) > 0 {
		return fmt.Errorf("month aggregation verification failed: %v", monthErrors)
	}

	return nil
}

// calculateExpectedAggregations вычисляет ожидаемые агрегаты из событий
func calculateExpectedAggregations(events []*entity.Event) (
	map[string]map[entity.EventType]*aggData,
	map[string]map[entity.EventType]*aggData,
) {
	dayAggs := make(map[string]map[entity.EventType]*aggData)
	monthAggs := make(map[string]map[entity.EventType]*aggData)

	for _, event := range events {
		// Дневные агрегаты
		day := event.StartedAt.Truncate(24 * time.Hour)
		dayKey := day.Format("2006-01-02")
		if dayAggs[dayKey] == nil {
			dayAggs[dayKey] = make(map[entity.EventType]*aggData)
		}
		if dayAggs[dayKey][event.Type] == nil {
			dayAggs[dayKey][event.Type] = &aggData{}
		}
		updateAggData(dayAggs[dayKey][event.Type], event)

		// Месячные агрегаты
		month := time.Date(event.StartedAt.Year(), event.StartedAt.Month(), 1, 0, 0, 0, 0, event.StartedAt.Location())
		monthKey := month.Format("2006-01")
		if monthAggs[monthKey] == nil {
			monthAggs[monthKey] = make(map[entity.EventType]*aggData)
		}
		if monthAggs[monthKey][event.Type] == nil {
			monthAggs[monthKey][event.Type] = &aggData{}
		}
		updateAggData(monthAggs[monthKey][event.Type], event)
	}

	return dayAggs, monthAggs
}

type aggData struct {
	countEvents  int64
	sumDuration  int64
	sumBytesUp   int64
	sumBytesDown int64
}

func updateAggData(agg *aggData, event *entity.Event) {
	agg.countEvents++
	if event.DurationSec != nil {
		agg.sumDuration += int64(*event.DurationSec)
	}
	agg.sumBytesUp += event.BytesUp
	agg.sumBytesDown += event.BytesDown
}

func convertDayAggsToMap(aggs []*entity.DayAggregation) map[string]map[entity.EventType]*aggData {
	result := make(map[string]map[entity.EventType]*aggData)
	for _, agg := range aggs {
		dayKey := agg.Day.Format("2006-01-02")
		if result[dayKey] == nil {
			result[dayKey] = make(map[entity.EventType]*aggData)
		}
		result[dayKey][agg.Type] = &aggData{
			countEvents:  agg.CountEvents,
			sumDuration:  agg.SumDuration,
			sumBytesUp:   agg.SumBytesUp,
			sumBytesDown: agg.SumBytesDown,
		}
	}
	return result
}

func convertMonthAggsToMap(aggs []*entity.MonthAggregation) map[string]map[entity.EventType]*aggData {
	result := make(map[string]map[entity.EventType]*aggData)
	for _, agg := range aggs {
		monthKey := agg.Month.Format("2006-01")
		if result[monthKey] == nil {
			result[monthKey] = make(map[entity.EventType]*aggData)
		}
		result[monthKey][agg.Type] = &aggData{
			countEvents:  agg.CountEvents,
			sumDuration:  agg.SumDuration,
			sumBytesUp:   agg.SumBytesUp,
			sumBytesDown: agg.SumBytesDown,
		}
	}
	return result
}

func compareDayAggregations(expected, actual map[string]map[entity.EventType]*aggData) []string {
	var errors []string

	for dayKey, dayTypes := range expected {
		for eventType, expectedData := range dayTypes {
			actualData, exists := actual[dayKey][eventType]
			if !exists {
				errors = append(errors, fmt.Sprintf("missing day aggregation for %s/%s", dayKey, eventType))
				continue
			}

			if expectedData.countEvents != actualData.countEvents {
				errors = append(errors, fmt.Sprintf(
					"day %s/%s count mismatch: expected %d, got %d",
					dayKey, eventType, expectedData.countEvents, actualData.countEvents,
				))
			}
			if expectedData.sumDuration != actualData.sumDuration {
				errors = append(errors, fmt.Sprintf(
					"day %s/%s duration mismatch: expected %d, got %d",
					dayKey, eventType, expectedData.sumDuration, actualData.sumDuration,
				))
			}
			if expectedData.sumBytesUp != actualData.sumBytesUp {
				errors = append(errors, fmt.Sprintf(
					"day %s/%s bytesUp mismatch: expected %d, got %d",
					dayKey, eventType, expectedData.sumBytesUp, actualData.sumBytesUp,
				))
			}
			if expectedData.sumBytesDown != actualData.sumBytesDown {
				errors = append(errors, fmt.Sprintf(
					"day %s/%s bytesDown mismatch: expected %d, got %d",
					dayKey, eventType, expectedData.sumBytesDown, actualData.sumBytesDown,
				))
			}
		}
	}

	return errors
}

func compareMonthAggregations(expected, actual map[string]map[entity.EventType]*aggData) []string {
	var errors []string

	for monthKey, monthTypes := range expected {
		for eventType, expectedData := range monthTypes {
			actualData, exists := actual[monthKey][eventType]
			if !exists {
				errors = append(errors, fmt.Sprintf("missing month aggregation for %s/%s", monthKey, eventType))
				continue
			}

			if expectedData.countEvents != actualData.countEvents {
				errors = append(errors, fmt.Sprintf(
					"month %s/%s count mismatch: expected %d, got %d",
					monthKey, eventType, expectedData.countEvents, actualData.countEvents,
				))
			}
			if expectedData.sumDuration != actualData.sumDuration {
				errors = append(errors, fmt.Sprintf(
					"month %s/%s duration mismatch: expected %d, got %d",
					monthKey, eventType, expectedData.sumDuration, actualData.sumDuration,
				))
			}
			if expectedData.sumBytesUp != actualData.sumBytesUp {
				errors = append(errors, fmt.Sprintf(
					"month %s/%s bytesUp mismatch: expected %d, got %d",
					monthKey, eventType, expectedData.sumBytesUp, actualData.sumBytesUp,
				))
			}
			if expectedData.sumBytesDown != actualData.sumBytesDown {
				errors = append(errors, fmt.Sprintf(
					"month %s/%s bytesDown mismatch: expected %d, got %d",
					monthKey, eventType, expectedData.sumBytesDown, actualData.sumBytesDown,
				))
			}
		}
	}

	return errors
}
