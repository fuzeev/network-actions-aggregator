package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/internal/infrastructure/postgres"
	postgresRepo "network-actions-aggregator/internal/infrastructure/repository/postgres"
	"network-actions-aggregator/pkg/logger"
)

// Config конфигурация генератора нагрузки
type Config struct {
	// Kafka
	KafkaBrokers []string
	KafkaTopic   string

	// Database
	DBConfig postgres.Config

	// Load generation
	UserCount       int           // Количество уникальных пользователей
	EventsPerSecond int           // Целевая скорость генерации событий/сек
	Duration        time.Duration // Длительность теста (0 = бесконечно)
	BatchSize       int           // Размер батча для отправки в Kafka
	WorkersCount    int           // Количество воркеров для отправки

	// Verification
	VerifyInterval time.Duration // Интервал проверки событий в БД
}

// Metrics метрики производительности
type Metrics struct {
	eventsSent     atomic.Int64
	eventsVerified atomic.Int64
	eventsFailed   atomic.Int64
	bytesProduced  atomic.Int64

	startTime     time.Time
	mu            sync.RWMutex
	lastEventTime time.Time
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Загрузка переменных окружения
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	// Инициализация логгера
	log := logger.NewLogger()

	// Конфигурация
	config := Config{
		KafkaBrokers: []string{
			fmt.Sprintf("%s:%s",
				getEnv("KAFKA_HOST", "localhost"),
				getEnv("KAFKA_PORT", "9092")),
		},
		KafkaTopic: getEnv("KAFKA_TOPIC", "events.telecom"),

		DBConfig: postgres.Config{
			Host:            getEnv("POSTGRES_HOST", "localhost"),
			Port:            getEnv("POSTGRES_PORT", "5432"),
			User:            getEnv("POSTGRES_USER", "app_user"),
			Password:        getEnv("POSTGRES_PASSWORD", "app_password"),
			DBName:          getEnv("POSTGRES_DB", "network_events"),
			SSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},

		UserCount:       getEnvInt("GENERATOR_USER_COUNT", 1000),
		EventsPerSecond: getEnvInt("GENERATOR_EVENTS_PER_SEC", 1000),
		Duration:        parseDuration(getEnv("GENERATOR_DURATION", "60s"), 60*time.Second),
		BatchSize:       getEnvInt("GENERATOR_BATCH_SIZE", 100),
		WorkersCount:    getEnvInt("GENERATOR_WORKERS", 4),
		VerifyInterval:  parseDuration(getEnv("GENERATOR_VERIFY_INTERVAL", "10s"), 10*time.Second),
	}

	log.Info("load generator configuration", map[string]interface{}{
		"kafka_brokers":     config.KafkaBrokers,
		"kafka_topic":       config.KafkaTopic,
		"user_count":        config.UserCount,
		"events_per_second": config.EventsPerSecond,
		"duration":          config.Duration.String(),
		"batch_size":        config.BatchSize,
		"workers":           config.WorkersCount,
		"verify_interval":   config.VerifyInterval.String(),
	})

	// Подключение к базе данных для верификации
	db, err := postgres.NewConnection(config.DBConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	defer sqlDB.Close()

	log.Info("connected to database for verification", map[string]interface{}{
		"host":     config.DBConfig.Host,
		"database": config.DBConfig.DBName,
	})

	// Инициализация компонентов
	generator := NewEventGenerator(config.UserCount)
	producer := NewKafkaProducer(config.KafkaBrokers, config.KafkaTopic)
	defer producer.Close()

	eventRepo := postgresRepo.NewEventRepository(db)
	verifier := NewVerifier(eventRepo)

	// Метрики
	metrics := &Metrics{
		startTime:     time.Now(),
		lastEventTime: time.Now(),
	}

	// Контекст с graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Канал для событий
	eventsChan := make(chan []*entity.Event, config.WorkersCount*2)

	var wg sync.WaitGroup

	// Генератор событий
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventsChan)

		ticker := time.NewTicker(time.Second / time.Duration(config.EventsPerSecond/config.BatchSize))
		defer ticker.Stop()

		var testDuration <-chan time.Time
		if config.Duration > 0 {
			testDuration = time.After(config.Duration)
		}

		for {
			select {
			case <-ctx.Done():
				log.Info("event generation stopped", nil)
				return
			case <-testDuration:
				log.Info("test duration completed", nil)
				return
			case <-ticker.C:
				events := generator.GenerateBatch(config.BatchSize)
				select {
				case eventsChan <- events:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Воркеры для отправки в Kafka
	for i := 0; i < config.WorkersCount; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case events, ok := <-eventsChan:
					if !ok {
						return
					}

					// Отправка в Kafka
					start := time.Now()
					if err := producer.SendBatch(ctx, events); err != nil {
						log.Error("failed to send batch", map[string]interface{}{
							"worker": workerID,
							"error":  err,
							"size":   len(events),
						})
						metrics.eventsFailed.Add(int64(len(events)))
						continue
					}

					// Подсчет метрик
					metrics.eventsSent.Add(int64(len(events)))
					metrics.mu.Lock()
					metrics.lastEventTime = time.Now()
					metrics.mu.Unlock()

					// Отслеживание для верификации
					eventIDs := make([]uuid.UUID, len(events))
					for i, event := range events {
						eventIDs[i] = event.EventID
					}
					verifier.TrackSentBatch(eventIDs)

					latency := time.Since(start)
					if latency > 100*time.Millisecond {
						log.Warn("high kafka latency", map[string]interface{}{
							"worker":  workerID,
							"latency": latency.String(),
							"size":    len(events),
						})
					}
				}
			}
		}()
	}

	// Периодическая верификация
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(config.VerifyInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := verifier.VerifyAll(ctx); err != nil {
					log.Error("verification failed", map[string]interface{}{
						"error": err,
					})
				} else {
					stats := verifier.Stats()
					metrics.eventsVerified.Store(int64(stats.Verified))

					log.Info("verification completed", map[string]interface{}{
						"stats": stats.String(),
					})
				}
			}
		}
	}()

	// Периодический вывод метрик
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				printMetrics(log, metrics, producer)
			}
		}
	}()

	log.Info("load generator started", map[string]interface{}{
		"target_events_per_sec": config.EventsPerSecond,
	})

	// Ожидание сигнала завершения или окончания теста
	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", map[string]interface{}{
			"signal": sig,
		})
	case <-ctx.Done():
	}

	// Graceful shutdown
	cancel()

	// Ожидание завершения с таймаутом
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("all workers stopped", nil)
	case <-time.After(30 * time.Second):
		log.Warn("shutdown timeout exceeded", nil)
	}

	// Финальная верификация
	log.Info("performing final verification...", nil)
	if err := verifier.VerifyAll(ctx); err != nil {
		log.Error("final verification failed", map[string]interface{}{
			"error": err,
		})
	}

	// Финальные метрики
	printFinalReport(log, metrics, verifier)

	return nil
}

func printMetrics(log logger.Logger, m *Metrics, producer *KafkaProducer) {
	elapsed := time.Since(m.startTime)
	sent := m.eventsSent.Load()
	verified := m.eventsVerified.Load()
	failed := m.eventsFailed.Load()

	eventsPerSec := float64(sent) / elapsed.Seconds()

	kafkaStats := producer.Stats()

	log.Info("metrics", map[string]interface{}{
		"elapsed":         elapsed.Round(time.Second).String(),
		"events_sent":     sent,
		"events_verified": verified,
		"events_failed":   failed,
		"events_per_sec":  fmt.Sprintf("%.2f", eventsPerSec),
		"kafka_messages":  kafkaStats.Messages,
		"kafka_bytes":     kafkaStats.Bytes,
		"kafka_errors":    kafkaStats.Errors,
	})
}

func printFinalReport(log logger.Logger, m *Metrics, v *Verifier) {
	elapsed := time.Since(m.startTime)
	sent := m.eventsSent.Load()
	failed := m.eventsFailed.Load()

	stats := v.Stats()

	eventsPerSec := float64(sent) / elapsed.Seconds()

	log.Info("=== FINAL REPORT ===", map[string]interface{}{
		"total_duration":     elapsed.Round(time.Second).String(),
		"events_sent":        sent,
		"events_failed":      failed,
		"events_verified":    stats.Verified,
		"events_missing":     stats.Missing,
		"events_pending":     stats.Pending,
		"avg_events_per_sec": fmt.Sprintf("%.2f", eventsPerSec),
		"success_rate":       fmt.Sprintf("%.2f%%", stats.SuccessRate),
		"avg_latency":        stats.AvgLatency.Round(time.Millisecond).String(),
	})
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
