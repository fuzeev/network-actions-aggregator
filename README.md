# Network Actions Aggregator

Система агрегации событий мобильной сети (звонки, SMS, интернет-сессии).

## Архитектура

```
[Генератор/источник событий]
           │
           ▼
      [Redpanda]
           │
    (consumer-группы)
           │
           ├──────► [Ingestor] ──► [PostgreSQL (партиции)]
           │
           └──────► [Aggregator] ──► [Таблицы агрегатов]
                                        │
                                        └──► [Redis Cache]
                                                 ▲
                                                 │
                                         [API Service]
                                   (gRPC + HTTP gateway)
```

## Компоненты

- **Ingestor** - сервис приёма и записи событий в БД
- **Aggregator** - сервис агрегации по дням и месяцам
- **API** - gRPC/HTTP API для получения детализации и агрегатов
- **Generator** - генератор тестовых событий

## Быстрый старт

### 1. Установка зависимостей

```bash
# Копируем конфигурацию
cp .env.example .env

# Устанавливаем Go-зависимости
go mod download

# (Опционально) Устанавливаем инструменты для разработки
make install-tools
```

### 2. Запуск инфраструктуры

```bash
# Полная инициализация (запуск всех сервисов, создание топика, миграции)
make init

# Или по шагам:
make up                  # Запустить Docker контейнеры
make kafka-create-topic  # Создать топик events.telecom
make migrate-up          # Применить миграции БД
```

### 3. Проверка

```bash
# Проверить статус БД
make migrate-status

# Посмотреть логи
make logs

# Открыть Redpanda Console
open http://localhost:8080
```

## Доступные сервисы

После запуска `make init` доступны:

- **PostgreSQL**: `localhost:5432` (user: `app_user`, db: `network_events`)
- **Redis**: `localhost:6379`
- **Redpanda (Kafka)**: `localhost:9092`
- **Redpanda Console**: http://localhost:8080

## Разработка

### Структура проекта

```
.
├── cmd/                    # Точки входа приложений
│   ├── ingestor/          # Сервис приёма событий
│   ├── aggregator/        # Сервис агрегации
│   ├── api/               # API сервис
│   └── generator/         # Генератор событий
├── internal/              # Внутренний код
│   ├── domain/            # Доменный слой
│   │   ├── entity/        # Сущности
│   │   ├── repository/    # Интерфейсы репозиториев
│   │   └── service/       # Доменные сервисы
│   ├── usecase/           # Бизнес-логика
│   ├── repository/        # Реализации репозиториев
│   │   ├── postgres/      # PostgreSQL
│   │   └── cache/         # Redis
│   ├── infrastructure/    # Инфраструктура
│   │   └── kafka/         # Kafka consumer/producer
│   ├── handler/           # Обработчики
│   │   ├── grpc/          # gRPC handlers
│   │   └── http/          # HTTP handlers
│   └── config/            # Конфигурация
├── api/proto/             # Proto файлы
├── pkg/                   # Общие пакеты
│   ├── pb/                # Сгенерированный proto код
│   └── logger/            # Логирование
├── migrations/            # SQL миграции
└── docker-compose.yml     # Docker конфигурация
```

### Полезные команды

```bash
# Сборка
make build-all           # Собрать все сервисы
make build-ingestor      # Собрать только ingestor
make build-aggregator    # Собрать только aggregator
make build-api           # Собрать только api

# Тестирование
make test                # Запустить тесты
make test-coverage       # Покрытие тестами

# Работа с БД
make db-shell            # Подключиться к PostgreSQL
make migrate-up          # Применить миграции
make migrate-down        # Откатить миграции

# Работа с Kafka
make kafka-topics        # Список топиков
make kafka-describe-topic # Описание топика

# Утилиты
make redis-cli           # Подключиться к Redis
make logs                # Показать все логи
make clean               # Удалить все данные (ОСТОРОЖНО!)
make fmt                 # Форматировать код
make lint                # Запустить линтеры
```

### Proto файлы

Для генерации gRPC кода:

```bash
make proto-gen
```

## База данных

### Схема

- **events_raw** - партиционированная таблица сырых событий (по месяцам)
- **agg_day** - агрегаты по дням
- **agg_month** - агрегаты по месяцам

### Партиции

Партиции создаются автоматически при миграции на несколько месяцев вперёд.
В будущем нужно автоматизировать создание партиций (pg_partman или отдельная джоба).

## Kafka/Redpanda

### Топик: `events.telecom`

- **Партиции**: 8 (для параллельной обработки)
- **Ключ сообщения**: `user_id` (события одного пользователя идут в одну партицию)
- **Формат**: JSON

### Пример сообщения

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "source_id": "provider-1",
  "user_id": "123456789",
  "type": "CALL_OUT",
  "started_at": "2025-10-20T12:34:56Z",
  "ended_at": "2025-10-20T12:56:00Z",
  "duration_sec": 127,
  "bytes_up": 0,
  "bytes_down": 0,
  "meta": {"cell": "A1"}
}
```

### Типы событий

- `CALL_OUT` - исходящий звонок
- `CALL_IN` - входящий звонок
- `SMS_SENT` - отправка SMS
- `SMS_RECEIVED` - получение SMS
- `DATA` - интернет-сессия

## API

### gRPC

Порт: `9090`

Методы:
- `GetUserDetail` - детализация по пользователю
- `GetGlobalAgg` - агрегаты по всем пользователям

### HTTP Gateway

Порт: `8000`

Эндпоинты:
- `GET /api/v1/users/{user_id}/detail` - детализация
- `GET /api/v1/aggregates` - агрегаты

## Генератор нагрузки

Для нагрузочного тестирования системы создан генератор, который:
- ✅ Генерирует реалистичные события с рандомизацией
- ✅ Отправляет их в Kafka с настраиваемой скоростью
- ✅ Проверяет что все события записались в БД
- ✅ Фиксирует метрики производительности (throughput, latency, success rate)

### Запуск генератора

```bash
# Быстрый тест: 1000 событий/сек, 1 минута
go run cmd/generator/*.go

# Высокая нагрузка: 5000 событий/сек, 5 минут
export GENERATOR_EVENTS_PER_SEC=5000
export GENERATOR_DURATION=5m
export GENERATOR_WORKERS=8
go run cmd/generator/*.go
```

### Конфигурация

Через переменные окружения (см. `.env.example`):

```bash
GENERATOR_USER_COUNT=1000          # Количество уникальных пользователей
GENERATOR_EVENTS_PER_SEC=1000      # Целевая скорость (событий/сек)
GENERATOR_DURATION=60s             # Длительность теста (0 = бесконечно)
GENERATOR_BATCH_SIZE=100           # Размер батча для Kafka
GENERATOR_WORKERS=4                # Количество воркеров отправки
GENERATOR_VERIFY_INTERVAL=10s      # Интервал проверки БД
```

### Метрики генератора

Генератор выводит метрики каждые 5 секунд:

```
events_sent:      10000      # Всего отправлено в Kafka
events_verified:  9950       # Проверено в БД
events_failed:    0          # Ошибки отправки
events_per_sec:   1000.50    # Фактическая скорость
success_rate:     99.75%     # Процент успешности
avg_latency:      250ms      # Средняя задержка до БД
```

## Производительность

### Профилирование (pprof)

Каждый сервис экспонирует pprof на своём порту:
- Ingestor: `localhost:6060`
- Aggregator: `localhost:6061`
- API: `localhost:6062`

Пример использования:
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Надёжность

- **Дедубликация**: по уникальному ключу `(source_id, event_id)`
- **At-least-once delivery**: подтверждение чтения из Kafka только после записи в БД
- **Graceful shutdown**: корректное завершение работы с сохранением offset'ов

## TODO / Известные ограничения

- [ ] Автоматическое создание партиций (сейчас создаются вручную при миграции)
- [ ] Метрики Prometheus
- [ ] Distributed tracing (Jaeger/Tempo)
- [ ] Rate limiting для API
- [ ] Аутентификация/авторизация
- [ ] Health checks для сервисов
- [ ] CI/CD pipeline
- [ ] Helm charts для Kubernetes

## Лицензия

MIT

