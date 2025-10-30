package main

import (
	"context"
	"fmt"
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
