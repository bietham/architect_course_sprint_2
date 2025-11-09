# CinemaAbyss Events Service (Go + Kafka)

MVP сервис для проверки интеграции Kafka:
- Producer создаёт события в топиках `events.user`, `events.payment`, `events.movie`.
- Consumer (group) читает эти топики и пишет в лог; хранит последнее сообщение по типу в памяти.
- Простой HTTP API для генерации событий и чтения последнего обработанного.

## Переменные окружения
- `PORT` (по умолчанию `8082`)
- `KAFKA_BROKERS` (по умолчанию `kafka:9092`, через запятую)
- `KAFKA_GROUP_ID` (по умолчанию `events-service-group`)
- `TOPIC_USER` / `TOPIC_PAYMENT` / `TOPIC_MOVIE` (по умолчанию `events.user` / `events.payment` / `events.movie`)
- `PRODUCE_TIMEOUT_MS` (по умолчанию `5000`)
- `KAFKA_CONSUME_START` — `latest|first` (по умолчанию `latest`)

## HTTP API
- `POST /api/events/user` — создать событие пользователя
- `POST /api/events/payment` — создать событие платежа
- `POST /api/events/movie` — создать событие фильма
  - Тело (необязательно): `{ "key": "optional", "any": "json" }`
  - Ответ: `{"status":"queued","topic":"...","payload":{...}}`
- `GET /api/events/last/{type}` — вернуть последнее обработанное событие (`user|payment|movie`)
- `GET /healthz`, `GET /readyz`, `GET /__config`

## docker-compose (фрагмент)
```yaml
  events-service:
    build:
      context: ./src/microservices/events
      dockerfile: Dockerfile
    container_name: cinemaabyss-events-service
    depends_on:
      - kafka
    environment:
      PORT: 8082
      KAFKA_BROKERS: kafka:9092
      KAFKA_GROUP_ID: events-service-group
      TOPIC_USER: events.user
      TOPIC_PAYMENT: events.payment
      TOPIC_MOVIE: events.movie
    ports:
      - "8082:8082"
    networks:
      - cinemaabyss-network
```

## Примеры запросов
```bash
curl -s -X POST http://localhost:8082/api/events/user -H "content-type: application/json" -d '{"login":"neo"}' | jq
curl -s -X POST http://localhost:8082/api/events/payment -H "content-type: application/json" -d '{"amount": 42}' | jq
curl -s -X POST http://localhost:8082/api/events/movie -H "content-type: application/json" -d '{"title": "Matrix"}' | jq

curl -s http://localhost:8082/api/events/last/user | jq
```

## Проверка
- Запустите `docker compose up -d --build events-service` (Kafka должна быть в том же compose).
- Откройте Kafka UI: `http://localhost:8090`, убедитесь в наличии топиков `events.user`, `events.payment`, `events.movie`.
- Запустите тесты курса: `npm run test:local` из `tests/postman`.

## Примечание
Сервис логирует как публикацию, так и потребление сообщений. В логах контейнера вы увидите строки вида:
```
[producer] topic=events.user key= value={"type":"user","ts":"..."}
[consumer] topic=events.user key= value={"type":"user","ts":"..."}
```
