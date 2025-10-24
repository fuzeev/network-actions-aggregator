package kafka

import "time"

// ConsumerConfig конфигурация Kafka consumer
type ConsumerConfig struct {
	Brokers        []string
	Topic          string
	GroupID        string
	MaxWait        time.Duration
	MinBytes       int
	MaxBytes       int
	CommitInterval time.Duration
}

// ProducerConfig конфигурация Kafka producer
type ProducerConfig struct {
	Brokers      []string
	Topic        string
	Async        bool
	BatchSize    int
	BatchTimeout time.Duration
}
