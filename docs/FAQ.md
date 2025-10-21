# FAQ: Ответы на вопросы по архитектуре

## 1. Почему repository теперь в infrastructure?

**ИСПРАВЛЕНО!** Я переместил `internal/repository/` → `internal/infrastructure/repository/`

### Новая структура:
```
internal/
├── domain/                    # Интерфейсы и бизнес-логика
│   └── repository/           # Только интерфейсы!
│       └── event.go          # type EventRepository interface {...}
│
├── usecase/                   # Используют интерфейсы из domain
│
└── infrastructure/            # ВСЕ внешние зависимости
    ├── repository/            # Реализации интерфейсов
    │   ├── postgres/         # implements domain.EventRepository
    │   └── cache/            # implements domain.CacheRepository
    └── kafka/                 # Kafka consumer/producer
```

### Почему так лучше?
✅ **Логичнее**: все внешние зависимости в одном месте  
✅ **Чище**: infrastructure = "всё что снаружи нашего приложения"  
✅ **Согласованность**: repository и kafka на одном уровне абстракции  

## 2. Handler - это слой презентации? Как всё соединено?

**Да, handler = Presentation Layer.**

### Полная схема соединения компонентов:

```
┌──────────────────────────────────────────────────────────────┐
│                     cmd/ingestor/main.go                     │
│                  (Dependency Injection Container)            │
│                                                              │
│  func main() {                                               │
│    // 1. Инфраструктура                                     │
│    db := postgres.Connect(cfg.PostgresDSN)                  │
│    kafkaConsumer := kafka.NewConsumer(cfg.Brokers)          │
│                                                              │
│    // 2. Repositories (implements domain interfaces)        │
│    eventRepo := postgres.NewEventRepo(db)                   │
│                                                              │
│    // 3. Domain Services                                    │
│    dedupSvc := service.NewDeduplication(eventRepo)          │
│                                                              │
│    // 4. Use Cases                                          │
│    ingestUC := usecase.NewIngestEvents(eventRepo, dedupSvc) │
│                                                              │
│    // 5. Handler (использует use case)                     │
│    handler := func(msg) {                                    │
│      event := unmarshal(msg)                                │
│      ingestUC.Execute(ctx, event)                           │
│    }                                                         │
│                                                              │
│    // 6. Запуск                                             │
│    kafkaConsumer.Consume(ctx, handler)                      │
│  }                                                            │
└──────────────────────────────────────────────────────────────┘
```

### Поток данных:

```
[Kafka] → Consumer → Handler → UseCase → Repository → [DB]
                        ↓          ↓
                    Validation  Domain Logic
```

Подробная схема создана в файле `docs/FLOW.md` - там есть полный пример с кодом!

## 3. Партиции - нужно постоянно создавать?

### ✅ Решение: Автоматические партиции

Я создал миграцию `000002_auto_partition_function.up.sql` с SQL-функциями:

```sql
-- Создаёт партиции автоматически на N месяцев вперёд
SELECT ensure_partitions_exist(6);
```

### 3 способа использования:

**1. При старте приложения (РЕКОМЕНДУЮ):**
```go
// cmd/ingestor/main.go
func main() {
    db := connectDB()
    
    // Создаём партиции при старте
    _, err := db.Exec(ctx, "SELECT ensure_partitions_exist(3)")
    if err != nil {
        log.Warn("Failed to create partitions", err)
    }
    
    // ... запуск сервиса
}
```

**2. Через cron (раз в неделю):**
```bash
# В crontab
0 0 * * 0 docker-compose exec -T postgres psql -U app_user -d network_events -c "SELECT ensure_partitions_exist(3);"
```

**3. Вручную через Makefile:**
```bash
make auto-partitions  # Создаст партиции на 6 месяцев
```

### Почему партиции нужны?

**С партициями:**
- Запрос на 1 день = сканирует 1 партицию (~30GB)
- Удаление старых данных = DROP TABLE (мгновенно)
- Индексы маленькие и быстрые

**Без партиций:**
- Запрос на 1 день = сканирует всю таблицу (~1TB+)
- Удаление = медленный DELETE с VACUUM
- Индексы огромные

### Можно ли без партиций?

Да, но ТОЛЬКО если:
- Небольшой объём данных (< 100M записей)
- Не нужна высокая производительность
- События живут недолго (< 1 месяца)

Для продакшена с миллионами событий - **партиции обязательны**.

## 4. Что за папка pkg/?

### pkg/ = Публичный переиспользуемый код

```
pkg/
├── pb/              # Сгенерированный protobuf (используют все сервисы)
├── logger/          # Обёртка над zap/zerolog
└── utils/           # Хелперы (time, validation, etc)

internal/            # Приватный код (ТОЛЬКО для этого проекта)
├── domain/
├── usecase/
└── infrastructure/
```

### Правило:
- Код в `pkg/` можно импортировать извне:
  ```go
  import "github.com/you/project/pkg/logger"
  ```

- Код в `internal/` нельзя импортировать из других проектов (Go запретит)

### Когда использовать pkg/?

✅ **Да:**
- Protobuf типы (используют клиенты API)
- Логгер (может пригодиться в других проектах)
- Общие утилиты

❌ **Нет:**
- Бизнес-логика → `internal/domain`
- Use cases → `internal/usecase`
- Repositories → `internal/infrastructure/repository`

## 5. Разработка с Docker - как это работает?

### 🎯 ГЛАВНОЕ: Docker ТОЛЬКО для инфраструктуры!

```bash
# Режим разработки (РЕКОМЕНДУЮ):

# 1. Запускаем инфраструктуру в Docker
make up
# Запустятся: Postgres, Redis, Redpanda

# 2. Go-приложения запускаем ЛОКАЛЬНО
go run cmd/ingestor/main.go      # ← мгновенный перезапуск!
go run cmd/aggregator/main.go
go run cmd/api/main.go

# Они подключатся к localhost:5432, localhost:9092, etc.
```

### Преимущества локального запуска:

✅ **Мгновенные изменения** - просто Ctrl+C и перезапусти  
✅ **Отладка работает** - breakpoints, pprof  
✅ **Не нужно пересобирать Docker** - экономия времени  
✅ **Удобный hot-reload** - можно использовать air/reflex  

### Hot-reload (опционально):

```bash
# Установить air
go install github.com/cosmtrek/air@latest

# Запустить с автоперезагрузкой
make dev-ingestor    # Изменил код → автоматический рестарт
```

### Сборка Docker (только для продакшена):

```bash
# Соберём и запустим всё в Docker
docker-compose up --build

# Или раскомментируй сервисы в docker-compose.yml
```

### Типичный workflow:

```bash
# День 1: Инициализация
make init              # Запустит Postgres, Redis, Kafka, миграции

# Разработка
code internal/...      # Пишешь код
go run cmd/ingestor/main.go  # Тестируешь
make test              # Тесты

# Изменил код
Ctrl+C                 # Останови
go run cmd/ingestor/main.go  # Перезапусти (1 секунда!)

# Конец дня
make down              # Останови инфраструктуру (или оставь работать)

# Production deploy
docker-compose up --build  # Всё в контейнерах
```

### Подключение Go-приложений к Docker-инфраструктуре:

Go-приложение читает `.env` файл:
```env
POSTGRES_DSN=postgres://app_user:app_password@localhost:5432/network_events
KAFKA_BROKERS=localhost:9092
REDIS_ADDR=localhost:6379
```

Docker пробрасывает порты наружу:
```yaml
postgres:
  ports:
    - "5432:5432"  # ← localhost:5432 доступен
```

Всё просто работает! 🎉

## Дополнительные команды

```bash
# Посмотреть логи инфраструктуры
make logs

# Подключиться к БД
make db-shell

# Проверить топики Kafka
make kafka-topics

# Собрать бинарник
make build-ingestor
./bin/ingestor

# Запустить всё локально в разных терминалах
# Terminal 1:
make run-ingestor

# Terminal 2:
make run-aggregator

# Terminal 3:
make run-api
```

## Резюме изменений

✅ **Перемещён repository** в `internal/infrastructure/repository/`  
✅ **Созданы автоматические партиции** (миграция 000002)  
✅ **Добавлены команды для локального запуска** в Makefile  
✅ **Объяснена роль pkg/** (публичный код vs internal)  
✅ **Описан workflow разработки** без пересборки Docker  

## Следующие шаги

1. Запусти `make init` (когда Docker будет доступен)
2. Начни реализацию с domain entities
3. Используй `make run-ingestor` для локальной разработки
4. Смотри `docs/FLOW.md` для примеров интеграции компонентов

