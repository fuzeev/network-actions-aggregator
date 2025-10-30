package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"network-actions-aggregator/internal/domain/entity"
)

// EventGenerator генерирует случайные события для нагрузочного тестирования
type EventGenerator struct {
	rand         *rand.Rand
	sourceID     uuid.UUID
	userIDs      []uuid.UUID
	phoneNumbers []string
}

// NewEventGenerator создает новый генератор событий
func NewEventGenerator(userCount int) *EventGenerator {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	// Генерируем фиксированный набор пользователей для реалистичности
	userIDs := make([]uuid.UUID, userCount)
	phoneNumbers := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		userIDs[i] = uuid.New()
		phoneNumbers[i] = fmt.Sprintf("+7%010d", r.Int63n(10000000000))
	}

	return &EventGenerator{
		rand:         r,
		sourceID:     uuid.New(),
		userIDs:      userIDs,
		phoneNumbers: phoneNumbers,
	}
}

// GenerateEvent генерирует случайное событие
func (g *EventGenerator) GenerateEvent() *entity.Event {
	userIdx := g.rand.Intn(len(g.userIDs))
	eventType := g.randomEventType()

	event := &entity.Event{
		EventID:   uuid.New(),
		SourceID:  g.sourceID,
		UserID:    g.userIDs[userIdx],
		Phone:     g.phoneNumbers[userIdx],
		Type:      eventType,
		StartedAt: time.Now().Add(-time.Duration(g.rand.Intn(3600)) * time.Second),
		Meta:      g.generateMeta(),
		CreatedAt: time.Now(),
	}

	// Специфичные поля в зависимости от типа события
	switch eventType {
	case entity.EventTypeCallOut, entity.EventTypeCallIn:
		duration := g.rand.Intn(600) + 1 // 1-600 секунд
		event.DurationSec = &duration
		endTime := event.StartedAt.Add(time.Duration(duration) * time.Second)
		event.EndedAt = &endTime

	case entity.EventTypeSMSSent, entity.EventTypeSMSReceived:
		// SMS - мгновенные события
		event.EndedAt = &event.StartedAt

	case entity.EventTypeData:
		duration := g.rand.Intn(7200) + 60 // 60-7200 секунд
		event.DurationSec = &duration
		endTime := event.StartedAt.Add(time.Duration(duration) * time.Second)
		event.EndedAt = &endTime

		// Генерируем трафик (от 1KB до 500MB)
		event.BytesUp = int64(g.rand.Intn(50_000_000) + 1000)
		event.BytesDown = int64(g.rand.Intn(500_000_000) + 1000)
	}

	return event
}

// randomEventType возвращает случайный тип события с весами
func (g *EventGenerator) randomEventType() entity.EventType {
	// Распределение типов событий (реалистичное)
	// DATA - 60%, звонки - 25%, SMS - 15%
	roll := g.rand.Intn(100)

	switch {
	case roll < 60:
		return entity.EventTypeData
	case roll < 75:
		if g.rand.Intn(2) == 0 {
			return entity.EventTypeCallOut
		}
		return entity.EventTypeCallIn
	default:
		if g.rand.Intn(2) == 0 {
			return entity.EventTypeSMSSent
		}
		return entity.EventTypeSMSReceived
	}
}

// generateMeta генерирует случайные метаданные
func (g *EventGenerator) generateMeta() map[string]interface{} {
	cells := []string{"A1", "A2", "B1", "B2", "C1", "C2", "D1", "D2"}
	imeis := []string{
		"356938035643809",
		"490154203237518",
		"352933082395932",
		"358240051111110",
	}

	meta := map[string]interface{}{
		"cell": cells[g.rand.Intn(len(cells))],
		"imei": imeis[g.rand.Intn(len(imeis))],
	}

	// Иногда добавляем дополнительные поля
	if g.rand.Intn(10) < 3 {
		meta["signal_strength"] = g.rand.Intn(100)
	}
	if g.rand.Intn(10) < 2 {
		meta["roaming"] = true
	}

	return meta
}

// GenerateBatch генерирует пакет событий
func (g *EventGenerator) GenerateBatch(size int) []*entity.Event {
	events := make([]*entity.Event, size)
	for i := 0; i < size; i++ {
		events[i] = g.GenerateEvent()
	}
	return events
}
