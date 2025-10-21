#!/bin/bash

# Скрипт для создания партиций PostgreSQL на следующие месяцы
# Использование: ./scripts/create_partitions.sh [количество_месяцев]

set -e

# Переменные окружения
POSTGRES_USER=${POSTGRES_USER:-app_user}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-app_password}
POSTGRES_DB=${POSTGRES_DB:-network_events}
POSTGRES_HOST=${POSTGRES_HOST:-localhost}
POSTGRES_PORT=${POSTGRES_PORT:-5432}

# Количество месяцев для создания партиций
MONTHS_COUNT=${1:-6}

echo "Создание партиций на ближайшие $MONTHS_COUNT месяцев..."

for i in $(seq 0 $((MONTHS_COUNT - 1))); do
    # Получаем текущий и следующий месяц (macOS совместимый формат)
    CURRENT=$(date -v+${i}m "+%Y-%m")
    NEXT=$(date -v+$((i+1))m "+%Y-%m")

    PARTITION_NAME="events_raw_${CURRENT//-/_}"

    SQL="
    CREATE TABLE IF NOT EXISTS $PARTITION_NAME PARTITION OF events_raw
        FOR VALUES FROM ('$CURRENT-01') TO ('$NEXT-01');
    "

    echo "Создание партиции $PARTITION_NAME (с $CURRENT-01 по $NEXT-01)..."
    echo "$SQL" | docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
done

echo "✓ Партиции созданы успешно!"

