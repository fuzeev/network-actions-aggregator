# network-actions-aggregator

Шаблон high-load pet-проекта мобильного оператора (ingestion через брокер, ClickHouse, Redis). Это стартовый каркас без бизнес-логики.

## Быстрый старт (локально)
1. make init
2. make compose-up
3. make run-api (аналогично: make run-aggregator, make run-ingestor, make run-loadgen)

Сервисы:
- Redpanda Console: http://localhost:8080
- ClickHouse HTTP: http://localhost:8123

## Структура
```
cmd/
  api/          # REST/gRPC заглушка (пока только pprof+health)
  aggregator/   # консюмер (заглушка)
  ingestor/     # ingest сервис (заглушка)
  loadgen/      # продюсер нагрузки (заглушка)
internal/       # внутренняя логика (пусто)
proto/          # будущие .proto файлы
deploy/
  docker-compose.yml
  env/.env.example
```

## Makefile цели
- init, tidy, fmt, lint, vet, test, race
- compose-up / compose-down / compose-logs
- run-api / run-aggregator / run-ingestor / run-loadgen

## Переменные окружения (пример)
См. deploy/env/.env.example — при старте заглушки подхватывают переменные, не переопределяя уже заданные.

## Примечания
- pprof/health порты: api :6060, aggregator :6061, loadgen :6062, ingestor :6063
- Redpanda запускается с параметрами оптимизированными для локального dev.
