-- Создание основной партиционированной таблицы событий
CREATE TABLE IF NOT EXISTS events_raw (
    event_id      UUID NOT NULL,
    source_id     TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    type          TEXT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    ended_at      TIMESTAMPTZ,
    duration_sec  INTEGER,
    bytes_up      BIGINT DEFAULT 0,
    bytes_down    BIGINT DEFAULT 0,
    meta          JSONB DEFAULT '{}'::JSONB,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (source_id, event_id, started_at)
) PARTITION BY RANGE (DATE_TRUNC('month', started_at));

-- Создаём партиции на несколько месяцев вперёд и назад
-- Октябрь 2025
CREATE TABLE IF NOT EXISTS events_raw_2025_10 PARTITION OF events_raw
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE INDEX IF NOT EXISTS idx_events_raw_2025_10_user_started
    ON events_raw_2025_10 (user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_10_type_started
    ON events_raw_2025_10 (type, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_10_started
    ON events_raw_2025_10 (started_at);

-- Ноябрь 2025
CREATE TABLE IF NOT EXISTS events_raw_2025_11 PARTITION OF events_raw
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE INDEX IF NOT EXISTS idx_events_raw_2025_11_user_started
    ON events_raw_2025_11 (user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_11_type_started
    ON events_raw_2025_11 (type, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_11_started
    ON events_raw_2025_11 (started_at);

-- Декабрь 2025
CREATE TABLE IF NOT EXISTS events_raw_2025_12 PARTITION OF events_raw
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE INDEX IF NOT EXISTS idx_events_raw_2025_12_user_started
    ON events_raw_2025_12 (user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_12_type_started
    ON events_raw_2025_12 (type, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2025_12_started
    ON events_raw_2025_12 (started_at);

-- Январь 2026
CREATE TABLE IF NOT EXISTS events_raw_2026_01 PARTITION OF events_raw
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE INDEX IF NOT EXISTS idx_events_raw_2026_01_user_started
    ON events_raw_2026_01 (user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2026_01_type_started
    ON events_raw_2026_01 (type, started_at);
CREATE INDEX IF NOT EXISTS idx_events_raw_2026_01_started
    ON events_raw_2026_01 (started_at);

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

-- Таблица для отслеживания обработанных offset'ов Kafka
-- (для возможности ручного управления offset'ами при необходимости)
CREATE TABLE IF NOT EXISTS kafka_offsets (
    consumer_group TEXT NOT NULL,
    topic         TEXT NOT NULL,
    partition     INTEGER NOT NULL,
    offset_value  BIGINT NOT NULL,
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (consumer_group, topic, partition)
);

-- Комментарии к таблицам
COMMENT ON TABLE events_raw IS 'Основная партиционированная таблица сырых событий от мобильной сети';
COMMENT ON TABLE agg_day IS 'Агрегаты событий по дням и типам';
COMMENT ON TABLE agg_month IS 'Агрегаты событий по месяцам и типам';
COMMENT ON TABLE kafka_offsets IS 'Отслеживание offset''ов Kafka для каждой consumer группы';

-- ПРИМЕЧАНИЕ: В будущем нужно будет автоматизировать создание партиций
-- Можно использовать pg_partman или написать отдельную джобу

