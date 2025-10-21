.PHONY: help up down restart logs clean migrate-up migrate-down migrate-create test build create-partitions

# Цвета для вывода
CYAN := \033[0;36m
GREEN := \033[0;32m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Показать справку
	@echo "$(CYAN)Доступные команды:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'

# Docker команды
up: ## Запустить все сервисы
	docker compose up -d

down: ## Остановить все сервисы
	docker compose down

restart: down up ## Перезапустить все сервисы

logs: ## Показать логи всех сервисов
	docker compose logs -f

logs-postgres: ## Показать логи PostgreSQL
	docker compose logs -f postgres

logs-redis: ## Показать логи Redis
	docker compose logs -f redis

logs-redpanda: ## Показать логи Redpanda
	docker compose logs -f redpanda

# Загружаем переменные из .env
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Миграции (через goose)
GOOSE_DBSTRING := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=$(POSTGRES_SSLMODE)
GOOSE_MIGRATION_DIR := ./migrations

migrate-up: ## Применить все миграции
	@echo "$(CYAN)Применение миграций через goose...$(NC)"
	goose -dir $(GOOSE_MIGRATION_DIR) postgres "$(GOOSE_DBSTRING)" up
	@echo "$(GREEN)Миграции применены!$(NC)"

migrate-down: ## Откатить последнюю миграцию
	@echo "$(CYAN)Откат последней миграции через goose...$(NC)"
	goose -dir $(GOOSE_MIGRATION_DIR) postgres "$(GOOSE_DBSTRING)" down
	@echo "$(GREEN)Миграция откачена!$(NC)"

migrate-reset: ## Откатить все миграции
	@echo "$(RED)Откат всех миграций...$(NC)"
	goose -dir $(GOOSE_MIGRATION_DIR) postgres "$(GOOSE_DBSTRING)" reset
	@echo "$(GREEN)Все миграции откачены!$(NC)"

migrate-status: ## Проверить статус миграций
	@echo "$(CYAN)Статус миграций:$(NC)"
	goose -dir $(GOOSE_MIGRATION_DIR) postgres "$(GOOSE_DBSTRING)" status

migrate-create: ## Создать новую миграцию (использование: make migrate-create NAME=migration_name)
	@if [ -z "$(NAME)" ]; then \
		echo "$(RED)Ошибка: укажите NAME=migration_name$(NC)"; \
		exit 1; \
	fi
	@echo "$(CYAN)Создание новой миграции: $(NAME)$(NC)"
	goose -dir $(GOOSE_MIGRATION_DIR) create $(NAME) sql
	@echo "$(GREEN)Миграция создана!$(NC)"

# Дополнительные утилиты
db-shell: ## Подключиться к PostgreSQL
	docker compose exec postgres psql -U app_user -d network_events

redis-cli: ## Подключиться к Redis CLI
	docker compose exec redis redis-cli

kafka-topics: ## Показать список топиков Kafka
	docker compose exec redpanda rpk topic list

kafka-create-topic: ## Создать топик events.telecom
	docker compose exec redpanda rpk topic create events.telecom \
		--partitions 8 \
		--replicas 1 \
		--topic-config retention.ms=604800000
	@echo "$(GREEN)Топик создан!$(NC)"

kafka-describe-topic: ## Описание топика events.telecom
	docker compose exec redpanda rpk topic describe events.telecom

# Сборка и тесты
build-ingestor: ## Собрать ingestor
	@echo "$(CYAN)Сборка ingestor...$(NC)"
	go build -o bin/ingestor ./cmd/ingestor
	@echo "$(GREEN)Сборка завершена!$(NC)"

build-aggregator: ## Собрать aggregator
	@echo "$(CYAN)Сборка aggregator...$(NC)"
	go build -o bin/aggregator ./cmd/aggregator
	@echo "$(GREEN)Сборка завершена!$(NC)"

build-api: ## Собрать api
	@echo "$(CYAN)Сборка api...$(NC)"
	go build -o bin/api ./cmd/api
	@echo "$(GREEN)Сборка завершена!$(NC)"

build-generator: ## Собрать generator
	@echo "$(CYAN)Сборка generator...$(NC)"
	go build -o bin/generator ./cmd/generator
	@echo "$(GREEN)Сборка завершена!$(NC)"

build-all: build-ingestor build-aggregator build-api build-generator ## Собрать все сервисы

test: ## Запустить тесты
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Показать покрытие тестами
	go tool cover -html=coverage.out

# Proto
proto-gen: ## Сгенерировать код из proto файлов
	@mkdir -p pkg/pb
	protoc --go_out=pkg/pb --go_opt=paths=source_relative \
		--go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=pkg/pb --grpc-gateway_opt=paths=source_relative \
		api/proto/*.proto

# Линтеры и форматирование
lint: ## Запустить линтеры
	@echo "$(CYAN)Запуск линтеров...$(NC)"
	golangci-lint run ./...

fmt: ## Форматировать код
	@echo "$(CYAN)Форматирование кода...$(NC)"
	go fmt ./...
	goimports -w .

# Полный запуск
init: up kafka-create-topic migrate-up create-partitions

# Создать партиции PostgreSQL на ближайшие N месяцев (по умолчанию 6 месяцев)
create-partitions: ## Создать партиции PostgreSQL на ближайшие 6 месяцев
	@echo "$(CYAN)Создание партиций на ближайшие 6 месяцев...$(NC)"
	@./scripts/create_partitions.sh $(START_MONTH) 6

install-tools: ## Установить необходимые инструменты
	@echo "$(CYAN)Установка инструментов...$(NC)"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	@echo "$(GREEN)Инструменты установлены!$(NC)"

# Локальный запуск сервисов (для разработки)
run-ingestor: ## Запустить ingestor локально
	@echo "$(CYAN)Запуск ingestor...$(NC)"
	go run cmd/ingestor/main.go

run-aggregator: ## Запустить aggregator локально
	@echo "$(CYAN)Запуск aggregator...$(NC)"
	go run cmd/aggregator/main.go

run-api: ## Запустить api локально
	@echo "$(CYAN)Запуск api...$(NC)"
	go run cmd/api/main.go

run-generator: ## Запустить generator локально
	@echo "$(CYAN)Запуск generator...$(NC)"
	go run cmd/generator/main.go
