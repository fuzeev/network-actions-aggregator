# Примеры использования

## Быстрый старт

### 1. Инициализация проекта

```bash
# Клонировать репозиторий
cd /Users/o.fuzeev/GolandProjects/network-actions-aggregator

# Скопировать конфигурацию
cp .env.example .env

# Полная инициализация (Docker + миграции + топик Kafka)
make init
```

### 2. Проверка работы

```bash
# Проверить статус сервисов
docker-compose ps

# Посмотреть логи
make logs

# Проверить таблицы в БД
make migrate-status
```

### 3. Работа с Kafka

```bash
# Создать топик (если не создан автоматически)
make kafka-create-topic

# Посмотреть информацию о топике
make kafka-describe-topic

# Список всех топиков
make kafka-topics
```

## Примеры работы с API (после реализации сервисов)

### gRPC (через grpcurl)

```bash
# Установить grpcurl
brew install grpcurl

# Получить детализацию по пользователю
grpcurl -plaintext \
  -d '{
    "user_id": "123456789",
    "from": "2025-10-01T00:00:00Z",
    "to": "2025-10-31T23:59:59Z",
    "limit": 100
  }' \
  localhost:9090 \
  telecom.v1.UsageService/GetUserDetail

# Получить агрегаты по дням
grpcurl -plaintext \
  -d '{
    "granularity": "DAY",
    "from": "2025-10-01",
    "to": "2025-10-31"
  }' \
  localhost:9090 \
  telecom.v1.UsageService/GetGlobalAgg
```

### HTTP Gateway

```bash
# Детализация по пользователю
curl -X GET "http://localhost:8000/api/v1/users/123456789/detail?from=2025-10-01T00:00:00Z&to=2025-10-31T23:59:59Z&limit=100"

# Агрегаты по дням
curl -X GET "http://localhost:8000/api/v1/aggregates?granularity=DAY&from=2025-10-01&to=2025-10-31"

# Агрегаты по месяцам
curl -X GET "http://localhost:8000/api/v1/aggregates?granularity=MONTH&from=2025-01-01&to=2025-12-31"
```

## Отправка тестовых событий в Kafka

### Через rpk (Redpanda CLI)

```bash
# Отправить одно событие
docker-compose exec redpanda rpk topic produce events.telecom --key "123456789" <<EOF
{
  "event_id": "550e8400-e29b-41d4-a716-446655440001",
  "source_id": "provider-1",
  "user_id": "123456789",
  "type": "CALL_OUT",
  "started_at": "2025-10-21T10:30:00Z",
  "ended_at": "2025-10-21T10:35:00Z",
  "duration_sec": 300,
  "bytes_up": 0,
  "bytes_down": 0,
  "meta": {"cell": "A1", "imei": "123456789012345"}
}
EOF
```

### Через генератор (после реализации)

```bash
# Собрать генератор
make build-generator

# Запустить генератор (1000 событий в секунду)
./bin/generator --rate 1000 --duration 60s
```

## Работа с БД

### Подключение к PostgreSQL

```bash
# Через make
make db-shell

# Напрямую
docker-compose exec postgres psql -U app_user -d network_events
```

### Полезные SQL запросы

```sql
-- Посмотреть партиции
SELECT tablename FROM pg_tables WHERE tablename LIKE 'events_raw_%';

-- Количество событий по типам
SELECT type, COUNT(*) FROM events_raw GROUP BY type;

-- События за последний час
SELECT * FROM events_raw 
WHERE started_at > NOW() - INTERVAL '1 hour' 
ORDER BY started_at DESC 
LIMIT 10;

-- Агрегаты за сегодня
SELECT * FROM agg_day WHERE day = CURRENT_DATE;

-- Топ пользователей по количеству событий
SELECT user_id, COUNT(*) as event_count 
FROM events_raw 
WHERE started_at > NOW() - INTERVAL '7 days'
GROUP BY user_id 
ORDER BY event_count DESC 
LIMIT 10;
```

## Работа с Redis

```bash
# Подключиться к Redis
make redis-cli

# Посмотреть все ключи
KEYS *

# Посмотреть значение ключа детализации
GET user:123456789:detail:2025-10-01:2025-10-31:v1

# Посмотреть TTL ключа
TTL user:123456789:detail:2025-10-01:2025-10-31:v1

# Очистить весь кэш
FLUSHALL
```

## Профилирование

### CPU Profile

```bash
# Собрать CPU profile (30 секунд)
curl "http://localhost:6060/debug/pprof/profile?seconds=30" > cpu.prof

# Анализировать
go tool pprof cpu.prof

# В интерактивном режиме
(pprof) top10
(pprof) list функция_название
(pprof) web  # открыть граф в браузере
```

### Memory Profile

```bash
# Собрать memory profile
curl "http://localhost:6060/debug/pprof/heap" > mem.prof

# Анализировать
go tool pprof mem.prof
```

### Goroutines

```bash
# Посмотреть количество goroutine
curl "http://localhost:6060/debug/pprof/goroutine?debug=1"
```

## Мониторинг

### Проверка здоровья сервисов

```bash
# PostgreSQL
docker-compose exec postgres pg_isready -U app_user

# Redis
docker-compose exec redis redis-cli ping

# Redpanda
docker-compose exec redpanda rpk cluster health
```

### Логи

```bash
# Все логи
make logs

# Логи конкретного сервиса
make logs-postgres
make logs-redis
make logs-redpanda

# Фильтр логов по уровню (после реализации)
make logs | grep ERROR
make logs | grep WARN
```

## Создание дополнительных партиций

```bash
# Создать партиции на следующие 6 месяцев
./scripts/create_partitions.sh 2026-02 6

# Вручную через SQL
make db-shell
# Затем выполнить SQL из create_partitions.sh
```

## Очистка и перезапуск

```bash
# Перезапустить все сервисы
make restart

# Полная очистка (удалит все данные!)
make clean

# Переинициализация с нуля
make clean && make init
```

## Разработка

### Запуск тестов

```bash
# Все тесты
make test

# Покрытие
make test-coverage

# Только unit тесты
go test ./internal/domain/... ./internal/usecase/...

# Интеграционные тесты
go test ./tests/integration/...
```

### Генерация proto

```bash
# Установить инструменты
make install-tools

# Сгенерировать код из proto
make proto-gen
```

### Линтинг и форматирование

```bash
# Форматирование
make fmt

# Линтинг
make lint
```

## Troubleshooting

### Проблема: Postgres не стартует

```bash
# Проверить логи
make logs-postgres

# Проверить volume
docker volume ls | grep postgres

# Пересоздать с очисткой
make clean && make up
```

### Проблема: Redpanda не готов

```bash
# Подождать пока здоровье станет OK
docker-compose exec redpanda rpk cluster health

# Проверить порты
netstat -an | grep 9092
```

### Проблема: Миграции не применяются

```bash
# Применить миграции вручную
docker-compose exec postgres psql -U app_user -d network_events < migrations/000001_init_schema.up.sql

# Или через make
make migrate-up
```

### Проблема: Порты заняты

```bash
# Проверить какой процесс использует порт
lsof -i :5432
lsof -i :9092

# Изменить порты в docker-compose.yml
```

## Production Checklist

- [ ] Изменить пароли в .env
- [ ] Настроить автоматическое создание партиций
- [ ] Настроить мониторинг (Prometheus/Grafana)
- [ ] Настроить алерты
- [ ] Настроить backup БД
- [ ] Настроить retention policy для Kafka
- [ ] Настроить TLS для gRPC/HTTP
- [ ] Добавить аутентификацию
- [ ] Настроить rate limiting
- [ ] Провести нагрузочное тестирование

