# Архитектура проекта

## Чистая архитектура

Проект следует принципам Clean Architecture с разделением на слои:

```
┌─────────────────────────────────────────┐
│         Внешний слой (Frameworks)       │
│  cmd/, docker/, migrations/             │
│  ├─ Точки входа приложений              │
│  └─ Конфигурация инфраструктуры        │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│      Слой интерфейсов (Adapters)       │
│  internal/handler/, internal/repository/│
│  ├─ HTTP/gRPC handlers                  │
│  ├─ Postgres/Redis repositories         │
│  └─ Kafka consumers/producers           │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│     Слой бизнес-логики (Use Cases)     │
│  internal/usecase/                      │
│  ├─ Приём событий                       │
│  ├─ Агрегация                           │
│  └─ Выдача данных                       │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│      Доменный слой (Entities)          │
│  internal/domain/                       │
│  ├─ entity/ - сущности                  │
│  ├─ repository/ - интерфейсы            │
│  └─ service/ - доменные сервисы         │
└─────────────────────────────────────────┘
```

## Структура директорий

```
network-actions-aggregator/
│
├── cmd/                          # Точки входа приложений
│   ├── ingestor/                 # Сервис приёма событий
│   ├── aggregator/               # Сервис агрегации
│   ├── api/                      # API сервис (gRPC + HTTP)
│   └── generator/                # Генератор тестовых событий
│
├── internal/                     # Внутренний код приложения
│   │
│   ├── domain/                   # Доменный слой (центр архитектуры)
│   │   ├── entity/               # Доменные сущности
│   │   │   ├── event.go          # Event, EventType
│   │   │   ├── aggregation.go    # DayAgg, MonthAgg
│   │   │   └── user.go           # User (если нужно)
│   │   │
│   │   ├── repository/           # Интерфейсы репозиториев
│   │   │   ├── event.go          # EventRepository
│   │   │   ├── aggregation.go    # AggregationRepository
│   │   │   └── cache.go          # CacheRepository
│   │   │
│   │   └── service/              # Доменные сервисы
│   │       ├── deduplication.go  # Дедубликация событий
│   │       └── validation.go     # Валидация данных
│   │
│   ├── usecase/                  # Бизнес-логика (use cases)
│   │   ├── ingest_events.go      # UC: приём и сохранение событий
│   │   ├── aggregate_events.go   # UC: агрегация событий
│   │   ├── get_user_detail.go    # UC: получение детализации
│   │   └── get_aggregates.go     # UC: получение агрегатов
│   │
│   ├── infrastructure/           # Инфраструктурный слой (ВСЕ внешние зависимости)
│   │   ├── repository/           # Реализации репозиториев
│   │   │   ├── postgres/         # PostgreSQL репозитории
│   │   │   │   ├── event.go      # EventRepository impl
│   │   │   │   ├── aggregation.go # AggregationRepository impl
│   │   │   │   └── connection.go  # Подключение к БД
│   │   │   │
│   │   │   └── cache/            # Redis кэш
│   │   │       └── redis.go      # CacheRepository impl
│   │   │
│   │   └── kafka/                # Kafka/Redpanda
│   │       ├── consumer.go       # Consumer wrapper
│   │       ├── producer.go       # Producer wrapper
│   │       └── config.go         # Конфигурация Kafka
│   │
│   ├── handler/                  # Обработчики внешних запросов
│   │   ├── grpc/                 # gRPC handlers
│   │   │   ├── usage_service.go  # UsageService impl
│   │   │   └── interceptors.go   # Логирование, метрики
│   │   │
│   │   └── http/                 # HTTP handlers (если нужны дополнительные)
│   │       └── health.go         # Health check endpoint
│   │
│   └── config/                   # Конфигурация приложения
│       ├── config.go             # Структуры конфигурации
│       └── load.go               # Загрузка из env
│
├── pkg/                          # Публичные переносимые пакеты
│   ├── pb/                       # Сгенерированный protobuf код
│   ├── logger/                   # Логгер (zap/zerolog)
│   └── utils/                    # Утилиты общего назначения
│
├── api/                          # API спецификации
│   └── proto/                    # protobuf файлы
│       └── usage_service.proto
│
├── migrations/                   # SQL миграции
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
│
├── tests/                        # Тесты
│   ├── unit/                     # Юнит-тесты
│   ├── integration/              # Интеграционные тесты
│   └── load/                     # Нагрузочные тесты
│
├── scripts/                      # Вспомогательные скрипты
│   └── create_partitions.sh      # Создание партиций
│
├── docker/                       # Docker конфигурация
│   ├── Dockerfile.ingestor
│   ├── Dockerfile.aggregator
│   └── Dockerfile.api
│
├── docker-compose.yml            # Docker Compose для локальной разработки
├── Makefile                      # Команды для управления проектом
├── go.mod                        # Go модули
├── .env.example                  # Пример конфигурации
├── .gitignore
├── .golangci.yml                 # Конфигурация линтера
└── README.md                     # Документация
```

## Слои и их ответственность

### 1. Domain (internal/domain/)
**Правило**: Не зависит ни от чего. Чистая бизнес-логика.

- **entity/** - доменные объекты (Event, Aggregation)
- **repository/** - интерфейсы для работы с данными
- **service/** - доменные сервисы (валидация, дедубликация)

### 2. Use Cases (internal/usecase/)
**Правило**: Зависит только от domain. Оркестрирует бизнес-процессы.

- Приём событий с дедубликацией
- Агрегация событий по дням/месяцам
- Получение детализации с кэшированием
- Получение агрегатов

### 3. Repository (internal/repository/)
**Правило**: Реализует интерфейсы из domain/repository.

- **postgres/** - работа с PostgreSQL
- **cache/** - работа с Redis

### 4. Infrastructure (internal/infrastructure/)
**Правило**: Внешние зависимости (Kafka, очереди).

- Kafka consumer/producer
- Конфигурация подключений

### 5. Handler (internal/handler/)
**Правило**: Адаптеры для внешних протоколов.

- gRPC handlers - преобразуют proto в domain entities
- HTTP handlers - REST endpoints

### 6. CMD (cmd/)
**Правило**: Точки входа. Собирают все вместе (DI).

- Инициализация зависимостей
- Запуск сервисов
- Graceful shutdown

## Поток данных

### Ingestor (приём событий)
```
Kafka → Consumer → UseCase(IngestEvents) → Repository(Postgres) → DB
                        ↓
                   Domain Service (Deduplication)
```

### Aggregator (агрегация)
```
Kafka → Consumer → UseCase(AggregateEvents) → Repository(Postgres) → DB
                                                     ↓
                                             Cache Invalidation (Redis)
```

### API (выдача данных)
```
gRPC/HTTP Request → Handler → UseCase(GetDetail/GetAgg) → Cache? → Repository → DB
                                         ↓                    ↓
                                      Domain               Response
```

## Принципы

1. **Dependency Rule**: Зависимости направлены внутрь (к domain)
2. **Interface Segregation**: Маленькие целевые интерфейсы
3. **Dependency Injection**: Все зависимости инжектируются
4. **Single Responsibility**: Каждый модуль делает одно
5. **Open/Closed**: Расширяем через интерфейсы

## Тестирование

- **Domain/UseCase**: Юнит-тесты с моками репозиториев
- **Repository**: Интеграционные тесты с testcontainers
- **Handler**: Интеграционные тесты с реальными сервисами
- **E2E**: Docker compose + полный флоу

## TODO: Улучшения архитектуры

- [ ] Domain Events для связи между сервисами
- [ ] CQRS разделение для read/write
- [ ] Event Sourcing для аудита (опционально)
- [ ] Saga паттерн для распределённых транзакций

