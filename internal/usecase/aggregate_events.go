package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/domain/repository"
	"network-actions-aggregator/pkg/logger"
)

// AggregateEventsConfig конфигурация для usecase агрегации
type AggregateEventsConfig struct {
	// StreamingFlushInterval - интервал сброса накопленных агрегатов в БД
	StreamingFlushInterval time.Duration
	// LateEventThreshold - порог для late events (если событие старше этого, помечаем день как dirty)
	LateEventThreshold time.Duration
	// DirtyPeriodChannelSize - размер буфера канала для dirty periods
	DirtyPeriodChannelSize int
}

// dirtyPeriod описывает период, требующий пересчёта
type dirtyPeriod struct {
	day   *time.Time // nil если не нужно пересчитывать день
	month *time.Time // nil если не нужно пересчитывать месяц
}

// AggregateEventsUseCase обрабатывает агрегацию событий
type AggregateEventsUseCase struct {
	eventRepo       repository.EventRepository
	aggregationRepo repository.AggregationRepository
	config          AggregateEventsConfig
	logger          logger.Logger

	// In-memory буферы для streaming агрегации
	dayBuffer   map[string]map[entity.EventType]*dayAggregate
	monthBuffer map[string]map[entity.EventType]*monthAggregate
	bufferMu    sync.RWMutex

	// Канал для периодов, требующих пересчёта
	dirtyPeriodsChan chan dirtyPeriod

	// Трекинг обработанных событий для фильтрации дубликатов (в памяти последние N минут)
	processedEvents map[string]time.Time // eventID -> время обработки
	processedMu     sync.RWMutex
}

// NewAggregateEventsUseCase создает новый usecase агрегации
func NewAggregateEventsUseCase(
	eventRepo repository.EventRepository,
	aggregationRepo repository.AggregationRepository,
	config AggregateEventsConfig,
	logger logger.Logger,
) *AggregateEventsUseCase {
	if config.DirtyPeriodChannelSize <= 0 {
		config.DirtyPeriodChannelSize = 1000
	}

	return &AggregateEventsUseCase{
		eventRepo:        eventRepo,
		aggregationRepo:  aggregationRepo,
		config:           config,
		logger:           logger,
		dayBuffer:        make(map[string]map[entity.EventType]*dayAggregate),
		monthBuffer:      make(map[string]map[entity.EventType]*monthAggregate),
		dirtyPeriodsChan: make(chan dirtyPeriod, config.DirtyPeriodChannelSize),
		processedEvents:  make(map[string]time.Time),
	}
}

// Run запускает агрегатор с несколькими параллельными процессами
func (uc *AggregateEventsUseCase) Run(ctx context.Context, eventsChan <-chan *entity.Event) error {
	var wg sync.WaitGroup

	// Горутина 1: Обработка событий из Kafka
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := uc.processEvents(ctx, eventsChan); err != nil && err != context.Canceled {
			uc.logger.Error("event processing stopped with error", map[string]interface{}{
				"error": err,
			})
		}
	}()

	// Горутина 2: Streaming агрегация (flush буферов каждые N секунд)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := uc.runStreamingAggregation(ctx); err != nil && err != context.Canceled {
			uc.logger.Error("streaming aggregation stopped with error", map[string]interface{}{
				"error": err,
			})
		}
	}()

	// Горутина 3: Reconciliation (пересчёт "грязных" периодов из канала)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := uc.runReconciliation(ctx); err != nil && err != context.Canceled {
			uc.logger.Error("reconciliation stopped with error", map[string]interface{}{
				"error": err,
			})
		}
	}()

	// Горутина 4: Очистка старых записей из processedEvents
	wg.Add(1)
	go func() {
		defer wg.Done()
		uc.runProcessedEventsCleanup(ctx)
	}()

	wg.Wait()
	return ctx.Err()
}

// processEvents обрабатывает поток событий из Kafka (для streaming агрегации)
func (uc *AggregateEventsUseCase) processEvents(ctx context.Context, eventsChan <-chan *entity.Event) error {
	for {
		select {
		case <-ctx.Done():
			// Перед выходом сбрасываем буферы
			if err := uc.flushBuffers(ctx); err != nil {
				uc.logger.Error("failed to flush buffers on shutdown", map[string]interface{}{
					"error": err,
				})
			}
			return ctx.Err()

		case event, ok := <-eventsChan:
			if !ok {
				// Канал закрыт, сбрасываем буферы
				if err := uc.flushBuffers(ctx); err != nil {
					uc.logger.Error("failed to flush buffers on channel close", map[string]interface{}{
						"error": err,
					})
				}
				return nil
			}

			// Проверяем дубликаты
			if uc.isDuplicate(event) {
				uc.logger.Debug("skipping duplicate event", map[string]interface{}{
					"event_id": event.EventID,
				})
				continue
			}

			// Помечаем событие как обработанное
			uc.markProcessed(event)

			// Проверяем, не является ли событие "опоздавшим"
			if uc.isLateEvent(event) {
				uc.markDirty(event)
			}

			// Добавляем в in-memory буферы
			uc.addToBuffer(event)
		}
	}
}

// runStreamingAggregation периодически сбрасывает буферы в БД
func (uc *AggregateEventsUseCase) runStreamingAggregation(ctx context.Context) error {
	ticker := time.NewTicker(uc.config.StreamingFlushInterval)
	defer ticker.Stop()

	uc.logger.Info("starting streaming aggregation", map[string]interface{}{
		"flush_interval": uc.config.StreamingFlushInterval.String(),
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			if err := uc.flushBuffers(ctx); err != nil {
				uc.logger.Error("failed to flush buffers", map[string]interface{}{
					"error": err,
				})
			}
		}
	}
}

// runReconciliation обрабатывает "грязные" периоды из канала
func (uc *AggregateEventsUseCase) runReconciliation(ctx context.Context) error {
	uc.logger.Info("starting reconciliation worker", nil)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case period := <-uc.dirtyPeriodsChan:
			// Пересчитываем день, если указан
			if period.day != nil {
				dayStart := *period.day
				dayEnd := dayStart.Add(24 * time.Hour)

				if err := uc.reconcileDay(ctx, dayStart, dayEnd); err != nil {
					uc.logger.Error("failed to reconcile day", map[string]interface{}{
						"day":   dayStart.Format("2006-01-02"),
						"error": err,
					})
					// Не падаем, продолжаем работу
				} else {
					uc.logger.Info("reconciled dirty day", map[string]interface{}{
						"day": dayStart.Format("2006-01-02"),
					})
				}
			}

			// Пересчитываем месяц, если указан
			if period.month != nil {
				monthStart := *period.month
				monthEnd := monthStart.AddDate(0, 1, 0)

				if err := uc.reconcileMonth(ctx, monthStart, monthEnd); err != nil {
					uc.logger.Error("failed to reconcile month", map[string]interface{}{
						"month": monthStart.Format("2006-01"),
						"error": err,
					})
					// Не падаем, продолжаем работу
				} else {
					uc.logger.Info("reconciled dirty month", map[string]interface{}{
						"month": monthStart.Format("2006-01"),
					})
				}
			}
		}
	}
}

// isLateEvent проверяет, является ли событие "опоздавшим"
func (uc *AggregateEventsUseCase) isLateEvent(event *entity.Event) bool {
	eventAge := time.Since(event.StartedAt)
	return eventAge > uc.config.LateEventThreshold
}

// markDirty помечает день/месяц события как требующий пересчёта (отправляет в канал)
func (uc *AggregateEventsUseCase) markDirty(event *entity.Event) {
	day := event.StartedAt.Truncate(24 * time.Hour)
	month := time.Date(event.StartedAt.Year(), event.StartedAt.Month(), 1, 0, 0, 0, 0, event.StartedAt.Location())

	period := dirtyPeriod{
		day:   &day,
		month: &month,
	}

	// Неблокирующая отправка в канал
	select {
	case uc.dirtyPeriodsChan <- period:
		uc.logger.Debug("marked period as dirty due to late event", map[string]interface{}{
			"event_id":   event.EventID,
			"event_time": event.StartedAt,
			"day":        day.Format("2006-01-02"),
			"month":      month.Format("2006-01"),
		})
	default:
		uc.logger.Warn("dirty periods channel is full, dropping period", map[string]interface{}{
			"event_id": event.EventID,
			"day":      day.Format("2006-01-02"),
			"month":    month.Format("2006-01"),
		})
	}
}

// addToBuffer добавляет событие в in-memory буферы
func (uc *AggregateEventsUseCase) addToBuffer(event *entity.Event) {
	uc.bufferMu.Lock()
	defer uc.bufferMu.Unlock()

	// Добавляем в дневной буфер
	day := event.StartedAt.Truncate(24 * time.Hour)
	dayKey := day.Format("2006-01-02")

	if uc.dayBuffer[dayKey] == nil {
		uc.dayBuffer[dayKey] = make(map[entity.EventType]*dayAggregate)
	}
	if uc.dayBuffer[dayKey][event.Type] == nil {
		uc.dayBuffer[dayKey][event.Type] = &dayAggregate{
			day:       day,
			eventType: event.Type,
		}
	}

	agg := uc.dayBuffer[dayKey][event.Type]
	agg.countEvents++
	if event.DurationSec != nil {
		agg.sumDuration += int64(*event.DurationSec)
	}
	agg.sumBytesUp += event.BytesUp
	agg.sumBytesDown += event.BytesDown

	// Добавляем в месячный буфер
	month := time.Date(event.StartedAt.Year(), event.StartedAt.Month(), 1, 0, 0, 0, 0, event.StartedAt.Location())
	monthKey := month.Format("2006-01")

	if uc.monthBuffer[monthKey] == nil {
		uc.monthBuffer[monthKey] = make(map[entity.EventType]*monthAggregate)
	}
	if uc.monthBuffer[monthKey][event.Type] == nil {
		uc.monthBuffer[monthKey][event.Type] = &monthAggregate{
			month:     month,
			eventType: event.Type,
		}
	}

	monthAgg := uc.monthBuffer[monthKey][event.Type]
	monthAgg.countEvents++
	if event.DurationSec != nil {
		monthAgg.sumDuration += int64(*event.DurationSec)
	}
	monthAgg.sumBytesUp += event.BytesUp
	monthAgg.sumBytesDown += event.BytesDown
}

// flushBuffers сбрасывает накопленные буферы в БД
func (uc *AggregateEventsUseCase) flushBuffers(ctx context.Context) error {
	uc.bufferMu.Lock()

	// Копируем буферы чтобы быстро освободить lock
	dayBufferCopy := uc.dayBuffer
	monthBufferCopy := uc.monthBuffer

	// Создаём новые буферы
	uc.dayBuffer = make(map[string]map[entity.EventType]*dayAggregate)
	uc.monthBuffer = make(map[string]map[entity.EventType]*monthAggregate)

	uc.bufferMu.Unlock()

	// Если буферы пусты, нечего сбрасывать
	if len(dayBufferCopy) == 0 && len(monthBufferCopy) == 0 {
		return nil
	}

	uc.logger.Debug("flushing buffers", map[string]interface{}{
		"days":   len(dayBufferCopy),
		"months": len(monthBufferCopy),
	})

	// Сбрасываем дневные агрегаты
	for _, dayAggs := range dayBufferCopy {
		for _, agg := range dayAggs {
			if err := uc.aggregationRepo.UpsertDayAggregation(
				ctx,
				agg.day,
				agg.eventType,
				agg.countEvents,
				agg.sumDuration,
				agg.sumBytesUp,
				agg.sumBytesDown,
			); err != nil {
				return fmt.Errorf("failed to upsert day aggregation: %w", err)
			}
		}
	}

	// Сбрасываем месячные агрегаты
	for _, monthAggs := range monthBufferCopy {
		for _, agg := range monthAggs {
			if err := uc.aggregationRepo.UpsertMonthAggregation(
				ctx,
				agg.month,
				agg.eventType,
				agg.countEvents,
				agg.sumDuration,
				agg.sumBytesUp,
				agg.sumBytesDown,
			); err != nil {
				return fmt.Errorf("failed to upsert month aggregation: %w", err)
			}
		}
	}

	uc.logger.Info("buffers flushed successfully", map[string]interface{}{
		"days_flushed":   len(dayBufferCopy),
		"months_flushed": len(monthBufferCopy),
	})

	return nil
}

// isDuplicate проверяет, было ли событие уже обработано
func (uc *AggregateEventsUseCase) isDuplicate(event *entity.Event) bool {
	uc.processedMu.RLock()
	_, exists := uc.processedEvents[event.EventID.String()]
	uc.processedMu.RUnlock()
	return exists
}

// markProcessed помечает событие как обработанное
func (uc *AggregateEventsUseCase) markProcessed(event *entity.Event) {
	uc.processedMu.Lock()
	uc.processedEvents[event.EventID.String()] = time.Now()
	uc.processedMu.Unlock()
}

// runProcessedEventsCleanup периодически очищает старые записи из processedEvents
func (uc *AggregateEventsUseCase) runProcessedEventsCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	uc.logger.Info("starting processed events cleanup worker", nil)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			uc.cleanupProcessedEvents()
		}
	}
}

// cleanupProcessedEvents удаляет записи старше 1 часа
func (uc *AggregateEventsUseCase) cleanupProcessedEvents() {
	uc.processedMu.Lock()
	defer uc.processedMu.Unlock()

	threshold := time.Now().Add(-1 * time.Hour)
	count := 0

	for eventID, processedAt := range uc.processedEvents {
		if processedAt.Before(threshold) {
			delete(uc.processedEvents, eventID)
			count++
		}
	}

	if count > 0 {
		uc.logger.Debug("cleaned up processed events", map[string]interface{}{
			"removed":   count,
			"remaining": len(uc.processedEvents),
		})
	}
}

// reconcileDay пересчитывает агрегаты для конкретного дня из БД
func (uc *AggregateEventsUseCase) reconcileDay(ctx context.Context, dayStart, dayEnd time.Time) error {
	// Получаем все события за день
	events, err := uc.eventRepo.GetByTimeRange(ctx, dayStart, dayEnd)
	if err != nil {
		return fmt.Errorf("failed to get events for reconciliation: %w", err)
	}

	uc.logger.Debug("reconciling day from DB", map[string]interface{}{
		"day":          dayStart.Format("2006-01-02"),
		"events_count": len(events),
	})

	// Группируем по типам (пересчитываем с нуля, не инкрементально!)
	aggregates := make(map[entity.EventType]*dayAggregate)

	for _, event := range events {
		if aggregates[event.Type] == nil {
			aggregates[event.Type] = &dayAggregate{
				day:       dayStart,
				eventType: event.Type,
			}
		}

		agg := aggregates[event.Type]
		agg.countEvents++
		if event.DurationSec != nil {
			agg.sumDuration += int64(*event.DurationSec)
		}
		agg.sumBytesUp += event.BytesUp
		agg.sumBytesDown += event.BytesDown
	}

	// Для reconciliation используем REPLACE (удаляем старое, вставляем новое)
	// Это точнее чем UPSERT, т.к. пересчитываем с нуля
	for _, agg := range aggregates {
		if err := uc.aggregationRepo.UpsertDayAggregation(
			ctx,
			agg.day,
			agg.eventType,
			agg.countEvents,
			agg.sumDuration,
			agg.sumBytesUp,
			agg.sumBytesDown,
		); err != nil {
			return fmt.Errorf("failed to upsert day aggregation during reconciliation: %w", err)
		}
	}

	return nil
}

// reconcileMonth пересчитывает агрегаты для конкретного месяца из БД
func (uc *AggregateEventsUseCase) reconcileMonth(ctx context.Context, monthStart, monthEnd time.Time) error {
	// Получаем все события за месяц
	events, err := uc.eventRepo.GetByTimeRange(ctx, monthStart, monthEnd)
	if err != nil {
		return fmt.Errorf("failed to get events for reconciliation: %w", err)
	}

	uc.logger.Debug("reconciling month from DB", map[string]interface{}{
		"month":        monthStart.Format("2006-01"),
		"events_count": len(events),
	})

	// Группируем по типам
	aggregates := make(map[entity.EventType]*monthAggregate)

	for _, event := range events {
		if aggregates[event.Type] == nil {
			aggregates[event.Type] = &monthAggregate{
				month:     monthStart,
				eventType: event.Type,
			}
		}

		agg := aggregates[event.Type]
		agg.countEvents++
		if event.DurationSec != nil {
			agg.sumDuration += int64(*event.DurationSec)
		}
		agg.sumBytesUp += event.BytesUp
		agg.sumBytesDown += event.BytesDown
	}

	// Сохраняем пересчитанные агрегаты
	for _, agg := range aggregates {
		if err := uc.aggregationRepo.UpsertMonthAggregation(
			ctx,
			agg.month,
			agg.eventType,
			agg.countEvents,
			agg.sumDuration,
			agg.sumBytesUp,
			agg.sumBytesDown,
		); err != nil {
			return fmt.Errorf("failed to upsert month aggregation during reconciliation: %w", err)
		}
	}

	return nil
}

// dayAggregate промежуточная структура для агрегации по дням
type dayAggregate struct {
	day          time.Time
	eventType    entity.EventType
	countEvents  int64
	sumDuration  int64
	sumBytesUp   int64
	sumBytesDown int64
}

// monthAggregate промежуточная структура для агрегации по месяцам
type monthAggregate struct {
	month        time.Time
	eventType    entity.EventType
	countEvents  int64
	sumDuration  int64
	sumBytesUp   int64
	sumBytesDown int64
}
