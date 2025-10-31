package main

import (
	"context"
	"fmt"
	"network-actions-aggregator/internal/domain/repository"
	"os"
	"sync"
	"sync/atomic"
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
	TotalEvents     int // Фиксированное количество событий для генерации
	DuplicateEvents int // Количество дубликатов для генерации
	UserCount       int // Количество уникальных пользователей
	BatchSize       int // Размер батча для отправки в Kafka
	WorkersCount    int // Количество воркеров для отправки

	// Verification
	VerifyInterval      time.Duration // Интервал проверки событий в БД
	VerifyTimeout       time.Duration // Максимальное время ожидания записи в БД
	MaxVerifyIterations int           // Максимальное количество попыток проверки
}

// Metrics метрики производительности
type Metrics struct {
	eventsSent         atomic.Int64
	eventsVerified     atomic.Int64
	eventsFailed       atomic.Int64
	duplicatesSent     atomic.Int64
	duplicatesRejected atomic.Int64

	startTime time.Time
	endTime   time.Time
	mu        sync.RWMutex
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

		TotalEvents:         getEnvInt("GENERATOR_TOTAL_EVENTS", 100000),
		DuplicateEvents:     getEnvInt("GENERATOR_DUPLICATE_EVENTS", 1000),
		UserCount:           getEnvInt("GENERATOR_USER_COUNT", 1000),
		BatchSize:           getEnvInt("GENERATOR_BATCH_SIZE", 100),
		WorkersCount:        getEnvInt("GENERATOR_WORKERS", 4),
		VerifyInterval:      parseDuration(getEnv("GENERATOR_VERIFY_INTERVAL", "5s"), 5*time.Second),
		VerifyTimeout:       parseDuration(getEnv("GENERATOR_VERIFY_TIMEOUT", "5m"), 5*time.Minute),
		MaxVerifyIterations: getEnvInt("GENERATOR_MAX_VERIFY_ITERATIONS", 60),
	}

	log.Info("=== LOAD GENERATOR CONFIGURATION ===", map[string]interface{}{
		"kafka_brokers":         config.KafkaBrokers,
		"kafka_topic":           config.KafkaTopic,
		"total_events":          config.TotalEvents,
		"duplicate_events":      config.DuplicateEvents,
		"user_count":            config.UserCount,
		"batch_size":            config.BatchSize,
		"workers":               config.WorkersCount,
		"verify_interval":       config.VerifyInterval.String(),
		"verify_timeout":        config.VerifyTimeout.String(),
		"max_verify_iterations": config.MaxVerifyIterations,
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
	defer func() {
		_ = sqlDB.Close()
	}()

	log.Info("connected to database for verification", map[string]interface{}{
		"host":     config.DBConfig.Host,
		"database": config.DBConfig.DBName,
	})

	// Инициализация компонентов
	generator := NewEventGenerator(config.UserCount)
	producer := NewKafkaProducer(config.KafkaBrokers, config.KafkaTopic)
	defer func() {
		_ = producer.Close()
	}()

	eventRepo := postgresRepo.NewEventRepository(db)
	verifier := NewVerifier(eventRepo)

	// Метрики
	metrics := &Metrics{
		startTime: time.Now(),
	}

	ctx := context.Background()

	// Фаза 1: Генерация и отправка уникальных событий
	log.Info("=== PHASE 1: GENERATING UNIQUE EVENTS ===", map[string]interface{}{
		"count": config.TotalEvents,
	})

	uniqueEvents, err := generateAndSendEvents(ctx, log, config, generator, producer, verifier, metrics, config.TotalEvents)
	if err != nil {
		return fmt.Errorf("failed to generate and send unique events: %w", err)
	}

	log.Info("unique events sent to Kafka", map[string]interface{}{
		"sent":   metrics.eventsSent.Load(),
		"failed": metrics.eventsFailed.Load(),
	})

	// Фаза 2: Генерация и отправка дубликатов
	log.Info("=== PHASE 2: GENERATING DUPLICATE EVENTS ===", map[string]interface{}{
		"count": config.DuplicateEvents,
	})

	if err := generateAndSendDuplicates(ctx, log, config, producer, metrics, uniqueEvents, config.DuplicateEvents); err != nil {
		return fmt.Errorf("failed to generate and send duplicates: %w", err)
	}

	log.Info("duplicate events sent to Kafka", map[string]interface{}{
		"sent": metrics.duplicatesSent.Load(),
	})

	// Фаза 3: Ожидание и проверка записи в БД
	log.Info("=== PHASE 3: WAITING FOR DATABASE PERSISTENCE ===", nil)

	if err := waitForPersistence(ctx, log, config, verifier, metrics); err != nil {
		return fmt.Errorf("failed to verify persistence: %w", err)
	}

	// Фаза 4: Проверка дубликатов
	log.Info("=== PHASE 4: VERIFYING DUPLICATES WERE REJECTED ===", nil)

	if err := verifyDuplicatesRejected(log, verifier, metrics, config.TotalEvents); err != nil {
		return fmt.Errorf("failed to verify duplicates rejection: %w", err)
	}

	// Фаза 5: Проверка агрегаций
	log.Info("=== PHASE 5: VERIFYING AGGREGATIONS ===", nil)

	aggregationRepo := postgresRepo.NewAggregationRepository(db)
	if err := verifyAggregations(ctx, log, config, verifier, aggregationRepo, uniqueEvents); err != nil {
		return fmt.Errorf("failed to verify aggregations: %w", err)
	}

	// Финальный отчет
	metrics.mu.Lock()
	metrics.endTime = time.Now()
	metrics.mu.Unlock()

	printFinalReport(log, metrics, config)

	return nil
}

// generateAndSendEvents генерирует и отправляет уникальные события
func generateAndSendEvents(
	ctx context.Context,
	log logger.Logger,
	config Config,
	generator *EventGenerator,
	producer *KafkaProducer,
	verifier *Verifier,
	metrics *Metrics,
	totalEvents int,
) ([]*entity.Event, error) {
	allEvents := make([]*entity.Event, 0, totalEvents)
	var allEventsMu sync.Mutex

	eventsChan := make(chan []*entity.Event, config.WorkersCount*2)
	errorsChan := make(chan error, config.WorkersCount)

	var wg sync.WaitGroup

	// Воркеры для отправки в Kafka
	for i := 0; i < config.WorkersCount; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()

			for events := range eventsChan {
				if err := producer.SendBatch(ctx, events); err != nil {
					log.Error("failed to send batch", map[string]interface{}{
						"worker": workerID,
						"error":  err,
						"size":   len(events),
					})
					metrics.eventsFailed.Add(int64(len(events)))
					errorsChan <- err
					continue
				}

				metrics.eventsSent.Add(int64(len(events)))

				// Отслеживание для верификации
				eventIDs := make([]uuid.UUID, len(events))
				for i, event := range events {
					eventIDs[i] = event.EventID
				}
				verifier.TrackSentBatch(eventIDs)
			}
		}()
	}

	// Генерация и отправка событий батчами
	go func() {
		defer close(eventsChan)

		for i := 0; i < totalEvents; i += config.BatchSize {
			batchSize := config.BatchSize
			if i+batchSize > totalEvents {
				batchSize = totalEvents - i
			}

			events := generator.GenerateBatch(batchSize)

			allEventsMu.Lock()
			allEvents = append(allEvents, events...)
			allEventsMu.Unlock()

			select {
			case eventsChan <- events:
			case <-ctx.Done():
				return
			}

			// Логирование прогресса
			if (i+batchSize)%10000 == 0 || i+batchSize >= totalEvents {
				log.Info("generation progress", map[string]interface{}{
					"generated": i + batchSize,
					"total":     totalEvents,
					"progress":  fmt.Sprintf("%.1f%%", float64(i+batchSize)/float64(totalEvents)*100),
				})
			}
		}
	}()

	// Ожидание завершения отправки
	wg.Wait()
	close(errorsChan)

	// Проверка на ошибки
	if len(errorsChan) > 0 {
		return allEvents, fmt.Errorf("encountered %d errors during sending", len(errorsChan))
	}

	return allEvents, nil
}

// generateAndSendDuplicates отправляет дубликаты существующих событий
func generateAndSendDuplicates(
	ctx context.Context,
	log logger.Logger,
	config Config,
	producer *KafkaProducer,
	metrics *Metrics,
	originalEvents []*entity.Event,
	duplicateCount int,
) error {
	if len(originalEvents) == 0 {
		return fmt.Errorf("no original events to duplicate")
	}

	eventsChan := make(chan []*entity.Event, config.WorkersCount*2)
	errorsChan := make(chan error, config.WorkersCount)

	var wg sync.WaitGroup

	// Воркеры для отправки дубликатов
	for i := 0; i < config.WorkersCount; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()

			for events := range eventsChan {
				if err := producer.SendBatch(ctx, events); err != nil {
					log.Error("failed to send duplicate batch", map[string]interface{}{
						"worker": workerID,
						"error":  err,
						"size":   len(events),
					})
					errorsChan <- err
					continue
				}

				metrics.duplicatesSent.Add(int64(len(events)))
			}
		}()
	}

	// Генерация дубликатов
	go func() {
		defer close(eventsChan)

		for i := 0; i < duplicateCount; i += config.BatchSize {
			batchSize := config.BatchSize
			if i+batchSize > duplicateCount {
				batchSize = duplicateCount - i
			}

			batch := make([]*entity.Event, batchSize)
			for j := 0; j < batchSize; j++ {
				// Выбираем случайное событие из оригинальных
				idx := (i + j) % len(originalEvents)
				batch[j] = originalEvents[idx]
			}

			select {
			case eventsChan <- batch:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	close(errorsChan)

	if len(errorsChan) > 0 {
		return fmt.Errorf("encountered %d errors during sending duplicates", len(errorsChan))
	}

	return nil
}

// waitForPersistence ожидает записи всех событий в БД
func waitForPersistence(
	ctx context.Context,
	log logger.Logger,
	config Config,
	verifier *Verifier,
	metrics *Metrics,
) error {
	log.Info("waiting for all events to be persisted in database...", nil)

	ticker := time.NewTicker(config.VerifyInterval)
	defer ticker.Stop()

	timeout := time.After(config.VerifyTimeout)
	iteration := 0

	for {
		select {
		case <-timeout:
			stats := verifier.Stats()
			return fmt.Errorf("timeout waiting for persistence: verified %d/%d events",
				stats.Verified, stats.TotalSent)

		case <-ticker.C:
			iteration++

			if err := verifier.VerifyAll(ctx); err != nil {
				log.Error("verification failed", map[string]interface{}{
					"error":     err,
					"iteration": iteration,
				})
				continue
			}

			stats := verifier.Stats()
			metrics.eventsVerified.Store(int64(stats.Verified))

			log.Info("verification progress", map[string]interface{}{
				"iteration":    iteration,
				"sent":         stats.TotalSent,
				"verified":     stats.Verified,
				"missing":      stats.Missing,
				"pending":      stats.Pending,
				"success_rate": fmt.Sprintf("%.2f%%", stats.SuccessRate),
				"avg_latency":  stats.AvgLatency.Round(time.Millisecond).String(),
			})

			// Проверяем, все ли события записаны
			if stats.Verified == stats.TotalSent {
				log.Info("✓ ALL EVENTS SUCCESSFULLY PERSISTED!", map[string]interface{}{
					"total":       stats.TotalSent,
					"iterations":  iteration,
					"avg_latency": stats.AvgLatency.Round(time.Millisecond).String(),
				})
				return nil
			}

			// Проверяем максимальное количество попыток
			if iteration >= config.MaxVerifyIterations {
				return fmt.Errorf("max verification iterations reached: verified %d/%d events",
					stats.Verified, stats.TotalSent)
			}
		}
	}
}

// verifyDuplicatesRejected проверяет, что дубликаты НЕ записались в БД
func verifyDuplicatesRejected(
	log logger.Logger,
	verifier *Verifier,
	metrics *Metrics,
	expectedCount int,
) error {
	stats := verifier.Stats()

	// Количество уникальных событий в БД должно быть равно expectedCount
	if stats.Verified != expectedCount {
		return fmt.Errorf("unexpected event count in DB: expected %d, got %d",
			expectedCount, stats.Verified)
	}

	duplicatesRejected := metrics.duplicatesSent.Load()
	metrics.duplicatesRejected.Store(duplicatesRejected)

	log.Info("✓ DUPLICATES VERIFICATION PASSED!", map[string]interface{}{
		"duplicates_sent":     duplicatesRejected,
		"duplicates_rejected": duplicatesRejected,
		"unique_events_in_db": stats.Verified,
	})

	return nil
}

// verifyAggregations проверяет корректность агрегаций
func verifyAggregations(
	ctx context.Context,
	log logger.Logger,
	config Config,
	verifier *Verifier,
	aggregationRepo repository.AggregationRepository,
	events []*entity.Event,
) error {
	log.Info("waiting for aggregations to be computed...", map[string]interface{}{
		"events_count": len(events),
	})

	// Ждем некоторое время, чтобы агрегатор обработал события
	log.Info("waiting 30 seconds for aggregator to process events...", nil)
	time.Sleep(30 * time.Second)

	log.Info("verifying aggregations...", nil)

	if err := verifier.VerifyAggregations(ctx, aggregationRepo, events); err != nil {
		log.Error("aggregation verification failed", map[string]interface{}{
			"error": err,
		})
		return err
	}

	log.Info("✓ AGGREGATIONS VERIFICATION PASSED!", nil)
	return nil
}

// printFinalReport выводит финальный отчет о тестировании
func printFinalReport(log logger.Logger, metrics *Metrics, config Config) {
	totalDuration := metrics.endTime.Sub(metrics.startTime)
	sent := metrics.eventsSent.Load()
	failed := metrics.eventsFailed.Load()
	verified := metrics.eventsVerified.Load()
	duplicatesSent := metrics.duplicatesSent.Load()
	duplicatesRejected := metrics.duplicatesRejected.Load()

	eventsPerSec := float64(verified) / totalDuration.Seconds()

	log.Info("", nil)
	log.Info("╔═══════════════════════════════════════════════════════╗", nil)
	log.Info("║           FINAL PERFORMANCE REPORT                    ║", nil)
	log.Info("╚═══════════════════════════════════════════════════════╝", nil)
	log.Info("", nil)
	log.Info("📊 GENERATION STATS:", map[string]interface{}{
		"total_duration":   totalDuration.Round(time.Millisecond).String(),
		"unique_generated": config.TotalEvents,
		"unique_sent":      sent,
		"unique_failed":    failed,
		"unique_verified":  verified,
		"duplicates_sent":  duplicatesSent,
		"duplicates_in_db": 0, // Дубликаты должны быть отклонены
	})
	log.Info("", nil)
	log.Info("✅ VERIFICATION RESULTS:", map[string]interface{}{
		"expected_in_db":         config.TotalEvents,
		"actual_in_db":           verified,
		"all_unique_saved":       verified == int64(config.TotalEvents),
		"duplicates_rejected":    duplicatesRejected,
		"all_duplicates_blocked": duplicatesRejected == duplicatesSent,
	})
	log.Info("", nil)
	log.Info("⚡ PERFORMANCE METRICS:", map[string]interface{}{
		"events_per_second":    fmt.Sprintf("%.2f", eventsPerSec),
		"avg_time_per_event":   fmt.Sprintf("%.3f ms", 1000.0/eventsPerSec),
		"total_events_handled": sent + duplicatesSent,
	})
	log.Info("", nil)

	// Финальный вердикт
	success := verified == int64(config.TotalEvents) && duplicatesRejected == duplicatesSent && failed == 0
	if success {
		log.Info("🎉 TEST RESULT: SUCCESS! All requirements met.", nil)
	} else {
		log.Info("❌ TEST RESULT: FAILED! Some requirements not met.", nil)
	}
	log.Info("", nil)
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
