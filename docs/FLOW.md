# Архитектура слоёв и поток управления

## Слои Clean Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  PRESENTATION LAYER (Frameworks & Drivers)                      │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  cmd/ingestor/main.go                                     │  │
│  │  ├─ Создание зависимостей (DI container)                 │  │
│  │  ├─ Запуск Kafka consumer                                │  │
│  │  └─ Graceful shutdown                                     │  │
│  └───────────────────────────────────────────────────────────┘  │
│                            │                                     │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/handler/grpc/usage_service.go                   │  │
│  │  ├─ Принимает proto Request                              │  │
│  │  ├─ Валидирует входные данные                            │  │
│  │  ├─ Преобразует в domain entities                        │  │
│  │  └─ Возвращает proto Response                            │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  USE CASE LAYER (Application Business Rules)                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/usecase/ingest_events.go                        │  │
│  │  ├─ Оркестрация бизнес-процесса                          │  │
│  │  ├─ Вызов domain services                                │  │
│  │  ├─ Работа через repository interfaces                   │  │
│  │  └─ Обработка ошибок                                     │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  DOMAIN LAYER (Enterprise Business Rules) - ЦЕНТР!             │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/domain/entity/event.go                          │  │
│  │  ├─ Event struct                                          │  │
│  │  ├─ EventType enum                                        │  │
│  │  └─ Бизнес-методы (Validate, IsComplete, ...)           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/domain/repository/event.go                      │  │
│  │  └─ type EventRepository interface { Save(), Find() }    │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/domain/service/deduplication.go                 │  │
│  │  └─ IsDuplicate(event) bool                              │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  INFRASTRUCTURE LAYER (Frameworks & Drivers)                    │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/infrastructure/repository/postgres/event.go     │  │
│  │  ├─ implements EventRepository interface                  │  │
│  │  ├─ Работает с pgx драйвером                             │  │
│  │  └─ SQL запросы                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  internal/infrastructure/kafka/consumer.go                │  │
│  │  └─ Kafka consumer wrapper                                │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
                   [PostgreSQL, Kafka, Redis]
```

## Пример потока управления: Приём события

### 1. INGESTOR SERVICE

```
[Kafka Topic: events.telecom]
         │
         │ (JSON message)
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/infrastructure/kafka/consumer.go               │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func (c *Consumer) Consume(ctx context.Context)     │ │
│ │   for {                                              │ │
│ │     msg := reader.ReadMessage(ctx)                   │ │
│ │     handler.Handle(msg)  // ← вызов обработчика     │ │
│ │   }                                                   │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ cmd/ingestor/main.go                                    │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func handleMessage(msg kafka.Message) {             │ │
│ │   event := unmarshal(msg.Value)                     │ │
│ │   ingestUseCase.Execute(ctx, event) // ← use case  │ │
│ │   msg.Commit()                                       │ │
│ │ }                                                     │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/usecase/ingest_events.go                       │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func (uc *IngestUseCase) Execute(event *Event) {   │ │
│ │   // 1. Валидация через domain                     │ │
│ │   if err := event.Validate(); err != nil {         │ │
│ │     return err                                      │ │
│ │   }                                                  │ │
│ │                                                      │ │
│ │   // 2. Проверка дубликата (domain service)        │ │
│ │   if uc.dedupService.IsDuplicate(event) {          │ │
│ │     return ErrDuplicate                             │ │
│ │   }                                                  │ │
│ │                                                      │ │
│ │   // 3. Сохранение через repository interface      │ │
│ │   return uc.eventRepo.Save(ctx, event)             │ │
│ │ }                                                     │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/infrastructure/repository/postgres/event.go    │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func (r *EventRepo) Save(event *Event) error {     │ │
│ │   sql := `INSERT INTO events_raw (...)             │ │
│ │           VALUES ($1, $2, ...) ON CONFLICT DO...`  │ │
│ │   _, err := r.db.Exec(ctx, sql, event.ID, ...)    │ │
│ │   return err                                        │ │
│ │ }                                                     │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
    [PostgreSQL]
```

### 2. API SERVICE

```
[HTTP GET /api/v1/users/123/detail]
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/handler/http/middleware.go (опционально)       │
│ └─ Auth, Rate Limit, Logging                            │
└─────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/handler/grpc/usage_service.go                  │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func (s *Server) GetUserDetail(                     │ │
│ │   ctx, req *pb.UserDetailRequest                    │ │
│ │ ) (*pb.UserDetailResponse, error) {                 │ │
│ │                                                      │ │
│ │   // 1. Валидация входных данных                   │ │
│ │   if err := validateRequest(req); err != nil {     │ │
│ │     return nil, status.Error(codes.InvalidArg...)  │ │
│ │   }                                                  │ │
│ │                                                      │ │
│ │   // 2. Вызов use case                             │ │
│ │   events, err := s.getUserDetailUC.Execute(ctx,    │ │
│ │     req.UserId, req.From, req.To, req.Limit)       │ │
│ │                                                      │ │
│ │   // 3. Преобразование domain → proto              │ │
│ │   return toProtoResponse(events), nil              │ │
│ │ }                                                     │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ internal/usecase/get_user_detail.go                     │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ func (uc *GetUserDetailUC) Execute(...) ([]*Event) │ │
│ │   // 1. Проверить кэш                              │ │
│ │   cached := uc.cache.Get(cacheKey)                 │ │
│ │   if cached != nil {                                │ │
│ │     return cached                                   │ │
│ │   }                                                  │ │
│ │                                                      │ │
│ │   // 2. Запрос из БД                               │ │
│ │   events := uc.eventRepo.FindByUser(userId, ...)  │ │
│ │                                                      │ │
│ │   // 3. Сохранить в кэш                            │ │
│ │   uc.cache.Set(cacheKey, events, ttl)              │ │
│ │                                                      │ │
│ │   return events                                     │ │
│ │ }                                                     │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         │
         ▼
    [Redis] ←→ [PostgreSQL]
```

## Dependency Injection в main.go

```go
// cmd/ingestor/main.go
func main() {
    // 1. Загрузить конфигурацию
    cfg := config.Load()
    
    // 2. Создать подключения (infrastructure)
    db := postgres.Connect(cfg.PostgresDSN)
    kafkaConsumer := kafka.NewConsumer(cfg.KafkaBrokers)
    
    // 3. Создать repositories (infrastructure)
    eventRepo := postgres.NewEventRepository(db)
    
    // 4. Создать domain services
    dedupService := service.NewDeduplicationService(eventRepo)
    
    // 5. Создать use cases
    ingestUC := usecase.NewIngestEvents(eventRepo, dedupService)
    
    // 6. Создать handler с use case
    handler := func(msg kafka.Message) error {
        event := unmarshal(msg.Value)
        return ingestUC.Execute(ctx, event)
    }
    
    // 7. Запустить consumer
    kafkaConsumer.Consume(ctx, handler)
}
```

## Направление зависимостей

```
        cmd/ (main)
           │
           ├──────┐
           │      │
           ▼      ▼
      handler   infrastructure
           │      (implements)
           │         │
           ▼         │
        usecase ◄────┘
           │
           ▼
         domain
    (interfaces only)

Правило: Стрелки зависимостей ВСЕГДА указывают на domain!
```

