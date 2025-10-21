-- +goose Up
-- +goose StatementBegin
-- Создание основной партиционированной таблицы событий
CREATE TABLE IF NOT EXISTS events_raw (
    event_id      UUID NOT NULL,
    source_id     UUID NOT NULL,
    user_id       UUID NOT NULL,
    phone         TEXT NOT NULL,
    type          TEXT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    ended_at      TIMESTAMPTZ,
    duration_sec  INTEGER,
    bytes_up      BIGINT NOT NULL DEFAULT 0,
    bytes_down    BIGINT NOT NULL DEFAULT 0,
    meta          JSONB DEFAULT '{}'::JSONB,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (event_id, started_at)
) PARTITION BY RANGE (DATE_TRUNC('month', started_at));

-- Создаём индексы на ОСНОВНОЙ таблице
-- PostgreSQL автоматически создаст соответствующие индексы на всех партициях
CREATE INDEX IF NOT EXISTS idx_events_raw_user_started ON events_raw (user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_source_started ON events_raw (source_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_type_started ON events_raw (type, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_phone_started ON events_raw (phone, started_at);

-- Таблица агрегатов по дням
CREATE TABLE IF NOT EXISTS agg_day (
    day           DATE NOT NULL,
    type          TEXT NOT NULL,
    count_events  BIGINT NOT NULL DEFAULT 0,
    sum_duration  BIGINT NOT NULL DEFAULT 0,
    sum_bytes_up  BIGINT NOT NULL DEFAULT 0,
    sum_bytes_down BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (day, type)
);

CREATE INDEX IF NOT EXISTS idx_agg_day_day ON agg_day (day);
CREATE INDEX IF NOT EXISTS idx_agg_day_type ON agg_day (type);

-- Таблица агрегатов по месяцам
CREATE TABLE IF NOT EXISTS agg_month (
    month         DATE NOT NULL, -- первое число месяца
    type          TEXT NOT NULL,
    count_events  BIGINT NOT NULL DEFAULT 0,
    sum_duration  BIGINT NOT NULL DEFAULT 0,
    sum_bytes_up  BIGINT NOT NULL DEFAULT 0,
    sum_bytes_down BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (month, type)
);

CREATE INDEX IF NOT EXISTS idx_agg_month_month ON agg_month (month);
CREATE INDEX IF NOT EXISTS idx_agg_month_type ON agg_month (type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agg_month CASCADE;
DROP TABLE IF EXISTS agg_day CASCADE;
DROP TABLE IF EXISTS events_raw CASCADE;
-- +goose StatementEnd
