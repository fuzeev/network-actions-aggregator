package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/infrastructure/kafka"
	"network-actions-aggregator/internal/usecase"
	"network-actions-aggregator/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Инициализация логгера
	log := logger.NewLogger()

	// Конфигурация
	kafkaConfig := kafka.ConsumerConfig{
		Brokers:        []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
		Topic:          getEnv("KAFKA_TOPIC", "network-events"),
		GroupID:        getEnv("KAFKA_GROUP_ID", "ingestor-group"),
		MaxWait:        500 * time.Millisecond,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 1 * time.Second,
	}

	usecaseConfig := usecase.IngestEventsConfig{
		BatchSize:     getEnvInt("BATCH_SIZE", 500),
		BatchTimeout:  time.Duration(getEnvInt("BATCH_TIMEOUT_MS", 1000)) * time.Millisecond,
		WorkersCount:  getEnvInt("WORKERS_COUNT", 4),
		RetryAttempts: getEnvInt("RETRY_ATTEMPTS", 3),
		RetryDelay:    100 * time.Millisecond,
	}

	// TODO: Инициализация репозитория
	// Пока репозиторий не реализован, создаем заглушку для демонстрации
	// eventRepo := postgres.NewEventRepository(db, log)
	// ingestUC := usecase.NewIngestEventsUseCase(eventRepo, usecaseConfig, log)

	log.Info("NOTE: EventRepository not implemented yet. Ingestor will consume but not save events.")

	// Создание Kafka consumer
	consumer := kafka.NewConsumer(kafkaConfig, log)

	// Канал для событий (буфер для сглаживания нагрузки)
	eventsChan := make(chan *entity.Event, usecaseConfig.BatchSize*2)

	// Контекст с graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// WaitGroup для синхронизации
	var wg sync.WaitGroup

	// Запуск Kafka consumer в отдельной горутине
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := consumer.ConsumeEvents(ctx, eventsChan); err != nil {
			log.Error("kafka consumer error", "error", err)
		}
	}()

	// TODO: Запуск usecase для обработки событий (когда будет готов репозиторий)
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	if err := ingestUC.ProcessEvents(ctx, eventsChan); err != nil {
	// 		log.Error("usecase error", "error", err)
	// 	}
	// }()

	// Временный consumer для демонстрации
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for {
			select {
			case <-ctx.Done():
				log.Info("processed events", "total", count)
				return
			case event, ok := <-eventsChan:
				if !ok {
					log.Info("channel closed, processed events", "total", count)
					return
				}
				count++
				if count%100 == 0 {
					log.Info("processing events", "count", count, "last_event_id", event.EventID)
				}
			}
		}
	}()

	// Периодический вывод статистики
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := consumer.Stats()
				log.Info("kafka consumer stats",
					"messages", stats.Messages,
					"bytes", stats.Bytes,
					"lag", stats.Lag,
					"offset", stats.Offset)
			}
		}
	}()

	log.Info("ingestor started",
		"kafka_topic", kafkaConfig.Topic,
		"kafka_brokers", kafkaConfig.Brokers,
		"batch_size", usecaseConfig.BatchSize,
		"workers", usecaseConfig.WorkersCount)

	// Ожидание сигнала завершения
	sig := <-sigChan
	log.Info("received shutdown signal", "signal", sig)

	// Graceful shutdown
	cancel()

	// Ожидание завершения всех горутин с таймаутом
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("shutdown completed successfully")
	case <-time.After(30 * time.Second):
		log.Warn("shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout")
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}
