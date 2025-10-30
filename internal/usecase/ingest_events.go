package usecase

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/domain/repository"
	"network-actions-aggregator/pkg/logger"
)

// IngestEventsConfig конфигурация для usecase
type IngestEventsConfig struct {
	BatchSize     int
	BatchTimeout  time.Duration
	WorkersCount  int
	RetryAttempts int
	RetryDelay    time.Duration
}

// IngestEventsUseCase обрабатывает события из Kafka и сохраняет их пачками
type IngestEventsUseCase struct {
	eventRepo repository.EventRepository
	config    IngestEventsConfig
	logger    logger.Logger
}

// NewIngestEventsUseCase создает новый usecase
func NewIngestEventsUseCase(
	eventRepo repository.EventRepository,
	config IngestEventsConfig,
	logger logger.Logger,
) *IngestEventsUseCase {
	return &IngestEventsUseCase{
		eventRepo: eventRepo,
		config:    config,
		logger:    logger,
	}
}

// ProcessEvents читает события из канала и сохраняет их пачками
// Блокируется до закрытия канала или отмены контекста
func (uc *IngestEventsUseCase) ProcessEvents(ctx context.Context, eventsChan <-chan *entity.Event) error {
	group, ctx := errgroup.WithContext(ctx)

	// Запускаем несколько воркеров для параллельной обработки
	for i := 0; i < uc.config.WorkersCount; i++ {
		workerID := i
		group.Go(func() error {
			return uc.worker(ctx, workerID, eventsChan)
		})
	}

	// Ждем завершения всех воркеров
	if err := group.Wait(); err != nil {
		return err
	}

	return nil
}

// worker обрабатывает события из канала
func (uc *IngestEventsUseCase) worker(ctx context.Context, workerID int, eventsChan <-chan *entity.Event) error {
	batch := make([]*entity.Event, 0, uc.config.BatchSize)
	ticker := time.NewTicker(uc.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Сохраняем оставшиеся события перед выходом
			if len(batch) > 0 {
				if err := uc.saveBatchWithRetry(ctx, batch); err != nil {
					uc.logger.Error("failed to save remaining batch on shutdown", map[string]interface{}{
						"worker":     workerID,
						"batch_size": len(batch),
						"error":      err,
					})
					return err
				}
			}
			return ctx.Err()

		case event, ok := <-eventsChan:
			if !ok {
				// Канал закрыт, сохраняем оставшиеся события
				if len(batch) > 0 {
					if err := uc.saveBatchWithRetry(ctx, batch); err != nil {
						uc.logger.Error("failed to save remaining batch on channel close", map[string]interface{}{
							"worker":     workerID,
							"batch_size": len(batch),
							"error":      err,
						})
						return err
					}
				}
				return nil
			}

			// Валидируем событие
			if err := event.Validate(); err != nil {
				uc.logger.Warn("invalid event received", map[string]interface{}{
					"worker":   workerID,
					"event_id": event.EventID,
					"error":    err,
				})
				continue
			}

			batch = append(batch, event)

			// Сохраняем при достижении размера батча
			if len(batch) >= uc.config.BatchSize {
				if err := uc.saveBatchWithRetry(ctx, batch); err != nil {
					uc.logger.Error("failed to save batch", map[string]interface{}{
						"worker":     workerID,
						"batch_size": len(batch),
						"error":      err,
					})
					return err
				}
				batch = batch[:0] // Очищаем batch
				ticker.Reset(uc.config.BatchTimeout)
			}

		case <-ticker.C:
			// Сохраняем по таймауту, если есть события
			if len(batch) > 0 {
				if err := uc.saveBatchWithRetry(ctx, batch); err != nil {
					uc.logger.Error("failed to save batch on timeout", map[string]interface{}{
						"worker":     workerID,
						"batch_size": len(batch),
						"error":      err,
					})
					return err
				}
				batch = batch[:0] // Очищаем batch
			}
		}
	}
}

// saveBatchWithRetry сохраняет batch с повторными попытками
func (uc *IngestEventsUseCase) saveBatchWithRetry(ctx context.Context, batch []*entity.Event) error {
	var lastErr error

	for attempt := 0; attempt < uc.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(uc.config.RetryDelay * time.Duration(attempt)):
			}
		}

		if err := uc.eventRepo.BatchInsert(ctx, batch); err != nil {
			lastErr = err
			uc.logger.Warn("batch insert attempt failed", map[string]interface{}{
				"attempt":    attempt + 1,
				"batch_size": len(batch),
				"error":      err,
			})
			continue
		}

		uc.logger.Debug("batch saved successfully", map[string]interface{}{
			"batch_size": len(batch),
			"attempt":    attempt + 1,
		})
		return nil
	}

	return fmt.Errorf("failed to save batch after %d attempts: %w", uc.config.RetryAttempts, lastErr)
}

// ProcessSingleEvent обрабатывает одно событие (для тестов или простых случаев)
func (uc *IngestEventsUseCase) ProcessSingleEvent(ctx context.Context, event *entity.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	return uc.eventRepo.BatchInsert(ctx, []*entity.Event{event})
}
