# tg-channel-parser

Парсер публичных Telegram-каналов, разработанный для проекта **GameDevVideos**.

Проект автоматически собирает сообщения с YouTube-ссылками из Telegram-канала [GameDevVideos](https://t.me/s/GameDevVideos) и других каналов, сохраняет их в базу данных и предоставляет через REST API для сайта [rgd.chat/videos](https://rgd.chat/videos).

## Возможности

- Парсинг публичных Telegram-каналов через `t.me/s/<channel>`
- Фильтрация только YouTube-ссылок (`youtube.com`, `youtu.be`) — все остальные отбрасываются
- **Режим seed** — первичный сбор всех сообщений канала с пагинацией по всей истории
- **Режим delta** — инкрементальный сбор только новых сообщений (раз в 5 минут по cron)
- Отслеживание изменений: редактирование и удаление сообщений
- Отправка новых, изменённых и удалённых сообщений в Discord через webhook
- REST API для получения сообщений канала с пагинацией
- Admin UI от PocketBase для управления каналами и webhook-целями
- Контейнеризация через Docker

## Архитектура

```
┌─────────────┐     ┌──────────────┐     ┌───────────┐
│ Telegram    │────▶│ tg-channel-  │────▶│ PocketBase│
│ t.me/s/...  │     │ parser       │     │ (SQLite)  │
└─────────────┘     └──────────────┘     └───────────┘
                          │                    │
                          ▼                    ▼
                     ┌─────────┐         ┌──────────┐
                     │ Discord │         │ REST API │
                     │ Webhook │         │ /channel │
                     └─────────┘         │ /messages│
                                          └──────────┘
```

Приложение построено на **PocketBase** — он выступает в роли:
- HTTP-сервера и маршрутизатора
- ORM и базы данных (SQLite)
- Планировщика cron-задач
- Admin UI для управления данными

## Технологии

| Компонент | Технология |
|---|---|
| Язык | Go 1.26 |
| Фреймворк | PocketBase |
| Парсинг HTML | goquery |
| База данных | SQLite (через modernc.org/sqlite) |
| Discord | Webhooks API |
| Контейнеризация | Docker / Docker Compose |
| CI/CD | GitHub Actions → ghcr.io |

## Быстрый старт

### Предварительные требования

- Go 1.26+
- Docker (опционально)

### Запуск локально

```bash
# Установка зависимостей
go mod download

# Запуск (откроется Admin UI на http://localhost:8090/_/)
go run cmd/api/main.go serve
```

### Запуск через Docker

```bash
docker compose up -d
```

### Настройка после запуска

1. Откройте Admin UI по адресу `http://localhost:8090/_/`
2. Создайте аккаунт администратора
3. В коллекции **channels** добавьте канал `GameDevVideos` (username: `GameDevVideos`, enabled: `true`)
4. В коллекции **webhook_targets** добавьте URL Discord-вебхука (тип: `discord`)
5. В коллекции **channel_webhooks** свяжите канал и webhook

## API

### Получение сообщений канала

```
GET /channels/{channel}/messages?page=1&perPage=50
```

Параметры:
- `channel` — username канала (например, `GameDevVideos`) или ID записи в PocketBase
- `page` — номер страницы (по умолчанию: 1)
- `perPage` — элементов на странице (по умолчанию: 50, макс: 200)

Пример ответа:

```json
{
  "items": [
    {
      "id": 12345,
      "text": "Пример текста сообщения",
      "links": [
        {
          "url": "https://youtube.com/watch?v=...",
          "provider": "YouTube",
          "title": "Video Title",
          "description": "Video description",
          "thumbnail": "https://i.ytimg.com/vi/.../hqdefault.jpg"
        }
      ],
      "media": [],
      "views": 1500,
      "datetime": "2026-07-17T12:00:00+00:00",
      "edited": false
    }
  ],
  "page": 1,
  "perPage": 50,
  "total": 1234,
  "totalPages": 25
}
```

## Структура проекта

```
cmd/api/main.go              # Точка входа
internal/
├── api/handlers.go          # HTTP-обработчики
├── db/collections.go        # Инициализация коллекций PocketBase
├── discord/client.go        # Клиент Discord webhook
├── service/parser_service.go # Бизнес-логика парсинга
└── tme-parser/
    ├── client.go            # HTTP-клиент для t.me/s/
    ├── parser.go            # Парсинг HTML через goquery
    ├── parser_test.go       # Тесты парсера
    └── types.go             # Типы данных
compose.yml                  # Docker Compose
Dockerfile                   # Многостадийная сборка Docker
```

## Разработка

```bash
# Запуск в режиме разработки
make dev

# Запуск тестов
go test ./... -v

# Сборка
go build ./cmd/api/main.go
```

## CI/CD

При пуше в ветку `main` GitHub Actions автоматически:
1. Запускает сборку и тесты
2. В случае успеха собирает Docker-образ
3. Публикует образ в `ghcr.io/russian-gamedev/gd-videos.rgd.chat`

## Лицензия

MIT
