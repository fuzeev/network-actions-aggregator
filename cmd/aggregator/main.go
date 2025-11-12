package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
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

// Config конфигурация сервиса агрегатора
type Config struct {
	DBConfig postgres.Config

	// Kafka settings
	KafkaBrokers []string
	KafkaTopic   string
	KafkaGroupID string

	// Aggregation settings
	StreamingFlushInterval time.Duration // Как часто сбрасывать буферы
	ReconciliationInterval time.Duration // Интервал reconciliation (не используется, reconciliation идёт по требованию)
	LateEventThreshold     time.Duration // Порог для late events
	DirtyPeriodChannelSize int           // Размер буфера канала для dirty periods
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
		DBConfig: postgres.Config{
			Host:            getEnv("POSTGRES_HOST", "localhost"),
			Port:            getEnv("POSTGRES_PORT", "5432"),
			User:            getEnv("POSTGRES_USER", "app_user"),
			Password:        getEnv("POSTGRES_PASSWORD", "app_password"),
			DBName:          getEnv("POSTGRES_DB", "network_events"),
			SSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		KafkaBrokers: []string{
			fmt.Sprintf("%s:%s",
				getEnv("KAFKA_HOST", "localhost"),
				getEnv("KAFKA_PORT", "9092")),
		},
		KafkaTopic:             getEnv("KAFKA_TOPIC", "events.telecom"),
		KafkaGroupID:           getEnv("KAFKA_GROUP_ID", "aggregator-group"),
		StreamingFlushInterval: parseDuration(getEnv("AGGREGATOR_FLUSH_INTERVAL", "30s"), 30*time.Second),
		ReconciliationInterval: parseDuration(getEnv("AGGREGATOR_RECONCILIATION_INTERVAL", "1h"), 1*time.Hour),
		LateEventThreshold:     parseDuration(getEnv("AGGREGATOR_LATE_EVENT_THRESHOLD", "5m"), 5*time.Minute),
		DirtyPeriodChannelSize: getEnvInt("AGGREGATOR_DIRTY_CHANNEL_SIZE", 1000),
	}

	log.Info("=== AGGREGATOR SERVICE STARTING ===", map[string]interface{}{
		"postgres_host":        config.DBConfig.Host,
		"postgres_db":          config.DBConfig.DBName,
		"kafka_brokers":        config.KafkaBrokers,
		"kafka_topic":          config.KafkaTopic,
		"kafka_group_id":       config.KafkaGroupID,
		"streaming_flush":      config.StreamingFlushInterval.String(),
		"late_event_threshold": config.LateEventThreshold.String(),
		"dirty_channel_size":   config.DirtyPeriodChannelSize,
	})

	// Подключение к базе данных
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

	log.Info("connected to database", map[string]interface{}{
		"host":     config.DBConfig.Host,
		"database": config.DBConfig.DBName,
	})

	// Инициализация репозиториев
	eventRepo := postgresRepo.NewEventRepository(db)
	aggregationRepo := postgresRepo.NewAggregationRepository(db)

	// Инициализация Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers: config.KafkaBrokers,
		Topic:   config.KafkaTopic,
		GroupID: config.KafkaGroupID,
	}, log)
	if err != nil {
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}
	defer func() {
		_ = kafkaConsumer.Close()
	}()

	log.Info("kafka consumer initialized", map[string]interface{}{
		"brokers":  config.KafkaBrokers,
		"topic":    config.KafkaTopic,
		"group_id": config.KafkaGroupID,
	})

	// Инициализация usecase
	aggregateUseCase := usecase.NewAggregateEventsUseCase(
		eventRepo,
		aggregationRepo,
		usecase.AggregateEventsConfig{
			StreamingFlushInterval: config.StreamingFlushInterval,
			ReconciliationInterval: config.ReconciliationInterval,
			LateEventThreshold:     config.LateEventThreshold,
			DirtyPeriodChannelSize: config.DirtyPeriodChannelSize,
		},
		log,
	)

	// Контекст с отменой для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Канал для событий из Kafka
	eventsChan := make(chan *entity.Event, 1000)

	// Запускаем consumer в отдельной горутине
	errChan := make(chan error, 2)
	go func() {
		if err := kafkaConsumer.ConsumeToChannel(ctx, eventsChan); err != nil && err != context.Canceled {
			log.Error("kafka consumer stopped with error", map[string]interface{}{
				"error": err,
			})
			errChan <- err
		}
	}()

	go func() {
		if err := aggregateUseCase.Run(ctx, eventsChan); err != nil && err != context.Canceled {
			log.Error("aggregator stopped with error", map[string]interface{}{
				"error": err,
			})
			errChan <- err
		}
	}()

	log.Info("aggregator service started successfully", nil)

	// Ожидание завершения или ошибки
	select {
	case <-sigChan:
		log.Info("received shutdown signal", nil)
		cancel()
		// Даем время на завершение текущей агрегации
		time.Sleep(5 * time.Second)
		log.Info("aggregator service stopped", nil)
		return nil

	case err := <-errChan:
		log.Error("aggregator service failed", map[string]interface{}{
			"error": err,
		})
		cancel()
		return err
	}
}

// getEnv получает значение переменной окружения или возвращает значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt получает целочисленное значение переменной окружения
func getEnvInt(key string, defaultValue int) int {
	strValue := os.Getenv(key)
	if strValue == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(strValue)
	if err != nil {
		return defaultValue
	}
	return value
}

// parseDuration парсит duration из строки
func parseDuration(s string, defaultValue time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultValue
	}
	return d
}
