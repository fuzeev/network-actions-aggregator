package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/infrastructure/kafka"
	"network-actions-aggregator/internal/infrastructure/postgres"
	postgresRepo "network-actions-aggregator/internal/infrastructure/repository/postgres"
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
	// Загрузка переменных окружения из .env файла
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	// Инициализация логгера
	log := logger.NewLogger()

	// Конфигурация
	kafkaBroker := fmt.Sprintf("%s:%s",
		getEnv("KAFKA_HOST", "localhost"),
		getEnv("KAFKA_PORT", "9092"))

	kafkaConfig := kafka.ConsumerConfig{
		Brokers:        []string{kafkaBroker},
		Topic:          getEnv("KAFKA_TOPIC", "events.telecom"),
		GroupID:        getEnv("KAFKA_GROUP_ID_INGESTOR", "ingestor-group"),
		MaxWait:        500 * time.Millisecond,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 1 * time.Second,
	}

	usecaseConfig := usecase.IngestEventsConfig{
		BatchSize:     getEnvInt("INGESTOR_BATCH_SIZE", 1000),
		BatchTimeout:  parseDuration(getEnv("INGESTOR_BATCH_TIMEOUT", "5s"), 5*time.Second),
		WorkersCount:  getEnvInt("INGESTOR_WORKER_COUNT", 8),
		RetryAttempts: getEnvInt("RETRY_ATTEMPTS", 3),
		RetryDelay:    100 * time.Millisecond,
	}

	// Инициализация PostgreSQL
	dbConfig := postgres.Config{
		Host:            getEnv("POSTGRES_HOST", "localhost"),
		Port:            getEnv("POSTGRES_PORT", "5432"),
		User:            getEnv("POSTGRES_USER", "app_user"),
		Password:        getEnv("POSTGRES_PASSWORD", "app_password"),
		DBName:          getEnv("POSTGRES_DB", "network_events"),
		SSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
		MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: time.Duration(getEnvInt("POSTGRES_CONN_MAX_LIFETIME_MIN", 5)) * time.Minute,
		ConnMaxIdleTime: time.Duration(getEnvInt("POSTGRES_CONN_MAX_IDLE_TIME_MIN", 5)) * time.Minute,
	}

	db, err := postgres.NewConnection(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	defer sqlDB.Close()

	log.Info("connected to database", map[string]interface{}{
		"host":     dbConfig.Host,
		"port":     dbConfig.Port,
		"database": dbConfig.DBName,
	})

	// Инициализация репозитория и usecase
	eventRepo := postgresRepo.NewEventRepository(db)
	ingestUC := usecase.NewIngestEventsUseCase(eventRepo, usecaseConfig, log)

	// Создание Kafka consumer
	consumer, err := kafka.NewConsumer(kafkaConfig, log)
	if err != nil {
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}
	defer consumer.Close()

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
			log.Error("kafka consumer error", map[string]interface{}{
				"error": err,
			})
		}
	}()

	// Запуск usecase для обработки событий
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ingestUC.ProcessEvents(ctx, eventsChan); err != nil {
			log.Error("usecase error", map[string]interface{}{
				"error": err,
			})
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
				log.Info("kafka consumer stats", map[string]interface{}{
					"messages": stats.Messages,
					"bytes":    stats.Bytes,
					"lag":      stats.Lag,
					"offset":   stats.Offset,
				})
			}
		}
	}()

	log.Info("ingestor started", map[string]interface{}{
		"kafka_topic":   kafkaConfig.Topic,
		"kafka_brokers": kafkaConfig.Brokers,
		"batch_size":    usecaseConfig.BatchSize,
		"workers":       usecaseConfig.WorkersCount,
	})

	// Ожидание сигнала завершения
	sig := <-sigChan
	log.Info("received shutdown signal", map[string]interface{}{
		"signal": sig,
	})

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
		log.Info("shutdown completed successfully", nil)
	case <-time.After(30 * time.Second):
		log.Warn("shutdown timeout exceeded", nil)
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

func parseDuration(value string, defaultValue time.Duration) time.Duration {
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	return defaultValue
}
