#!/bin/bash

# Скрипт для создания партиций PostgreSQL на следующие месяцы
# Использование: ./scripts/create_partitions.sh YYYY-MM [количество_месяцев]

set -e

POSTGRES_DSN=${POSTGRES_DSN:-"postgres://app_user:app_password@localhost:5432/network_events?sslmode=disable"}
START_MONTH=${1:-$(date +%Y-%m)}
MONTHS_COUNT=${2:-3}

echo "Создание партиций начиная с $START_MONTH на $MONTHS_COUNT месяцев..."

for i in $(seq 0 $((MONTHS_COUNT - 1))); do
    CURRENT=$(date -j -v+${i}m -f "%Y-%m" "$START_MONTH" "+%Y-%m" 2>/dev/null || date -d "$START_MONTH +$i month" "+%Y-%m")
    NEXT=$(date -j -v+$((i+1))m -f "%Y-%m" "$START_MONTH" "+%Y-%m" 2>/dev/null || date -d "$START_MONTH +$((i+1)) month" "+%Y-%m")

    PARTITION_NAME="events_raw_${CURRENT//-/_}"

    SQL="
    CREATE TABLE IF NOT EXISTS $PARTITION_NAME PARTITION OF events_raw
        FOR VALUES FROM ('$CURRENT-01') TO ('$NEXT-01');

    CREATE INDEX IF NOT EXISTS idx_${PARTITION_NAME}_user_started
        ON $PARTITION_NAME (user_id, started_at);
    CREATE INDEX IF NOT EXISTS idx_${PARTITION_NAME}_type_started
        ON $PARTITION_NAME (type, started_at);
    CREATE INDEX IF NOT EXISTS idx_${PARTITION_NAME}_started
        ON $PARTITION_NAME (started_at);
    "

    echo "Создание партиции $PARTITION_NAME..."
    echo "$SQL" | docker-compose exec -T postgres psql -U app_user -d network_events
done

echo "✓ Партиции созданы успешно!"

