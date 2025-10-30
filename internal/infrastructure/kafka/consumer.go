package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/segmentio/kafka-go"
	"network-actions-aggregator/internal/domain/entity"
	"network-actions-aggregator/pkg/logger"
)

// Consumer - твоя реализация consumer с нуля
type Consumer struct {
	reader *kafka.Reader
	config ConsumerConfig
	logger logger.Logger
}

// NewConsumer создает новый Kafka consumer
func NewConsumer(config ConsumerConfig, logger logger.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupID,
		MaxWait:        config.MaxWait,
		MinBytes:       config.MinBytes,
		MaxBytes:       config.MaxBytes,
		CommitInterval: config.CommitInterval,
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader: reader,
		config: config,
		logger: logger,
	}
}

//todo узнать у нейросетки как еще можно доработать

// ConsumeEvents читает события из Kafka и отправляет в канал
func (c *Consumer) ConsumeEvents(ctx context.Context, eventsChan chan<- *entity.Event) error {
	defer close(eventsChan)

	c.logger.Info("starting kafka consumer", map[string]interface{}{
		"topic":    c.config.Topic,
		"group_id": c.config.GroupID,
		"brokers":  c.config.Brokers,
	})

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer stopped, context done", nil)
			return c.Close()
		default:
			msg, err := c.reader.FetchMessage(ctx)

			if err != nil {
				if errors.Is(err, context.Canceled) {
					c.logger.Info("consumer stopped, context done", nil)
					return c.Close()
				}

				c.logger.Error("unable to fetch message: "+err.Error(), nil)
				//TODO: реализовать паттерн Retry with Exponential Backoff, пробовать подключаться с таймаутами
				continue
			}

			event, err := c.parseEvent(msg.Value)
			if err != nil {
				c.logger.Error("unable to parse message", map[string]interface{}{
					"offset":    msg.Offset,
					"partition": msg.Partition,
					"error":     err,
				})

				//невалидные сообщения тоже коммитим чтобы не блочили нам очередь
				err = c.reader.CommitMessages(ctx, msg)
				if err != nil {
					c.logger.Error("unable to commit message: "+err.Error(), nil)
				}

				continue
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled")
			case eventsChan <- event:
				c.logger.Info("read and parsed event from kafka with id "+event.EventID.String(), nil)
			}

			err = c.reader.CommitMessages(ctx, msg)
			if err != nil {
				c.logger.Error("unable to commit message: "+err.Error(), nil)
				continue
			}
		}
	}
}

// parseEvent парсит JSON в Event
func (c *Consumer) parseEvent(data []byte) (*entity.Event, error) {
	var event entity.Event
	err := json.Unmarshal(data, &event)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

// Close закрывает consumer
func (c *Consumer) Close() error {
	err := c.reader.Close()
	if err != nil {
		return err
	}

	c.logger.Info("kafka reader stopped", nil)

	return nil
}

// Stats возвращает статистику
func (c *Consumer) Stats() kafka.ReaderStats {
	return c.reader.Stats()
}
