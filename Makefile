.PHONY: help up down restart logs clean migrate-up migrate-down migrate-create test build

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
	@echo "$(CYAN)Запуск сервисов...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Сервисы запущены!$(NC)"
	@echo "$(CYAN)Postgres:$(NC) localhost:5432"
	@echo "$(CYAN)Redis:$(NC) localhost:6379"
	@echo "$(CYAN)Redpanda:$(NC) localhost:9092"
	@echo "$(CYAN)Redpanda Console:$(NC) http://localhost:8080"

down: ## Остановить все сервисы
	@echo "$(CYAN)Остановка сервисов...$(NC)"
	docker-compose down

restart: down up ## Перезапустить все сервисы

logs: ## Показать логи всех сервисов
	docker-compose logs -f

logs-postgres: ## Показать логи PostgreSQL
	docker-compose logs -f postgres

logs-redis: ## Показать логи Redis
	docker-compose logs -f redis

logs-redpanda: ## Показать логи Redpanda
	docker-compose logs -f redpanda

clean: ## Удалить все контейнеры и volumes
	@echo "$(RED)Внимание! Это удалит все данные!$(NC)"
	@read -p "Вы уверены? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker-compose down -v; \
		echo "$(GREEN)Очистка завершена$(NC)"; \
	fi

# Миграции
migrate-up: ## Применить все миграции
	@echo "$(CYAN)Применение миграций...$(NC)"
	docker-compose exec -T postgres psql -U app_user -d network_events -f /docker-entrypoint-initdb.d/000001_init_schema.up.sql
	@echo "$(GREEN)Миграции применены!$(NC)"

migrate-down: ## Откатить миграции
	@echo "$(CYAN)Откат миграций...$(NC)"
	docker-compose exec -T postgres psql -U app_user -d network_events -f /docker-entrypoint-initdb.d/000001_init_schema.down.sql
	@echo "$(GREEN)Миграции откачены!$(NC)"

migrate-status: ## Проверить состояние БД
	@echo "$(CYAN)Проверка таблиц в БД...$(NC)"
	docker-compose exec postgres psql -U app_user -d network_events -c "\dt"

# Дополнительные утилиты
db-shell: ## Подключиться к PostgreSQL
	docker-compose exec postgres psql -U app_user -d network_events

redis-cli: ## Подключиться к Redis CLI
	docker-compose exec redis redis-cli

kafka-topics: ## Показать список топиков Kafka
	docker-compose exec redpanda rpk topic list

kafka-create-topic: ## Создать топик events.telecom
	@echo "$(CYAN)Создание топика events.telecom...$(NC)"
	docker-compose exec redpanda rpk topic create events.telecom \
		--partitions 8 \
		--replicas 1 \
		--topic-config retention.ms=604800000
	@echo "$(GREEN)Топик создан!$(NC)"

kafka-describe-topic: ## Описание топика events.telecom
	docker-compose exec redpanda rpk topic describe events.telecom

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
	@echo "$(CYAN)Запуск тестов...$(NC)"
	go test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN)Тесты завершены!$(NC)"

test-coverage: test ## Показать покрытие тестами
	go tool cover -html=coverage.out

# Proto
proto-gen: ## Сгенерировать код из proto файлов
	@echo "$(CYAN)Генерация proto...$(NC)"
	@mkdir -p pkg/pb
	protoc --go_out=pkg/pb --go_opt=paths=source_relative \
		--go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=pkg/pb --grpc-gateway_opt=paths=source_relative \
		api/proto/*.proto
	@echo "$(GREEN)Proto сгенерированы!$(NC)"

# Линтеры и форматирование
lint: ## Запустить линтеры
	@echo "$(CYAN)Запуск линтеров...$(NC)"
	golangci-lint run ./...

fmt: ## Форматировать код
	@echo "$(CYAN)Форматирование кода...$(NC)"
	go fmt ./...
	goimports -w .

# Полный запуск
init: up kafka-create-topic migrate-up auto-partitions ## Полная инициализация проекта
	@echo "$(GREEN)✓ Проект инициализирован и готов к работе!$(NC)"
	@echo ""
	@echo "$(CYAN)Доступные сервисы:$(NC)"
	@echo "  - PostgreSQL: localhost:5432 (user: app_user, db: network_events)"
	@echo "  - Redis: localhost:6379"
	@echo "  - Redpanda: localhost:9092"
	@echo "  - Redpanda Console: http://localhost:8080"
	@echo ""
	@echo "$(CYAN)Режим разработки (локальный запуск Go):$(NC)"
	@echo "  make run-ingestor      # Запустить ingestor локально"
	@echo "  make run-aggregator    # Запустить aggregator локально"
	@echo "  make run-api           # Запустить api локально"
	@echo "  make run-generator     # Запустить generator локально"
	@echo ""
	@echo "$(CYAN)Или соберите бинарники:$(NC)"
	@echo "  make build-all && ./bin/ingestor"

auto-partitions: ## Создать партиции автоматически
	@echo "$(CYAN)Создание партиций на 6 месяцев вперёд...$(NC)"
	docker-compose exec -T postgres psql -U app_user -d network_events -c "SELECT ensure_partitions_exist(6);"
	@echo "$(GREEN)Партиции созданы!$(NC)"

install-tools: ## Установить необходимые инструменты
	@echo "$(CYAN)Установка инструментов...$(NC)"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
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

# Режим разработки с hot-reload (требует air: go install github.com/cosmtrek/air@latest)
dev-ingestor: ## Запустить ingestor с hot-reload
	air -c .air.ingestor.toml

dev-aggregator: ## Запустить aggregator с hot-reload
	air -c .air.aggregator.toml

dev-api: ## Запустить api с hot-reload
	air -c .air.api.toml

