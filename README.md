# tgproxy

Минималистичный private proxy для Telegram Bot API на Go.

Сервис принимает запросы вида `/bot<token>/<method>`, проверяет `bot id` по allowlist и прозрачно проксирует разрешенные запросы на `https://api.telegram.org`.

## Что умеет

- проверяет `bot id` по `ALLOWED_BOT_IDS`
- возвращает `403 Forbidden` для неразрешенных ботов
- возвращает `400 Bad Request` для некорректного пути или токена
- проксирует `GET` и `POST` без ручной буферизации всего тела в память
- поддерживает `application/json`, `application/x-www-form-urlencoded`, `multipart/form-data`
- подходит для file upload, file download и long polling
- отдает `/healthz` локально

## Конфиг

Переменные окружения:

- `ALLOWED_BOT_IDS` — обязательный список разрешенных bot id через запятую, например `123456,987654`
- `LISTEN_ADDR` — полный адрес, например `:8080` или `0.0.0.0:8080`
- `PORT` — альтернатива `LISTEN_ADDR`, если нужен только порт
- `UPSTREAM_BASE_URL` — опционально, по умолчанию `https://api.telegram.org`

Приоритет конфигурации:

1. `LISTEN_ADDR`
2. `PORT`
3. значение по умолчанию `:8080`

## Локальный запуск

```bash
export ALLOWED_BOT_IDS=123456,987654
go run .
```

Проверка health endpoint:

```bash
curl http://127.0.0.1:8080/healthz
```

Пример запроса через proxy:

```bash
curl "http://127.0.0.1:8080/bot123456:SECRET/getMe"
```

## Docker

Сборка:

```bash
docker build -t tgproxy .
```

Запуск:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ALLOWED_BOT_IDS=123456,987654 \
  ghcr.io/attid/tgproxy:latest
```

## Docker Compose

Если нужно собирать контейнер прямо из репозитория `https://github.com/attid/tgproxy.git`, `docker-compose.yml` может выглядеть так:

```yaml
services:
  tgproxy:
    build: https://github.com/attid/tgproxy.git#main
    container_name: tgproxy
    restart: unless-stopped
    environment:
      ALLOWED_BOT_IDS: "123456,987654"
      LISTEN_ADDR: ":8080"
    ports:
      - "127.0.0.1:8080:8080"
    healthcheck:
      test: ["CMD", "/tgproxy", "healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 5s
```

Если используешь уже опубликованный образ, вместо `build` можно указать:

```yaml
services:
  tgproxy:
    image: ghcr.io/attid/tgproxy:latest
    container_name: tgproxy
    restart: unless-stopped
    environment:
      ALLOWED_BOT_IDS: "123456,987654"
      LISTEN_ADDR: ":8080"
    ports:
      - "127.0.0.1:8080:8080"
    healthcheck:
      test: ["CMD", "/tgproxy", "healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 5s
```

## Reverse Proxy

Этот контейнер предполагает, что TLS завершается снаружи, например в Caddy, Nginx или Traefik. Сам `tgproxy` слушает обычный HTTP внутри контейнера.

Схема:

1. внешний reverse proxy принимает HTTPS на вашем домене
2. reverse proxy проксирует трафик в `tgproxy`
3. `tgproxy` проверяет `bot id` и отправляет запрос в Telegram Bot API

## Разработка

Тесты:

```bash
go test ./...
```
