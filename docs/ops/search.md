# Запуск Search

Search принимает события заявок из Kafka и сохраняет их в своей PostgreSQL-базе. Поиск предложений ещё не запускается. [Гарантии и ограничения](../adr/0006-search-inbox.md).

## Локально

Добавьте в `.env` два разных пароля `SEARCH_DATABASE_OWNER_PASSWORD` и `SEARCH_DATABASE_APP_PASSWORD` по образцу `.env.example`.

```sh
make db-up
make search-db-init
make search-migrate
make kafka-up
```

В отдельных терминалах запускаются `make publish-outbox` и `make search`. Cabinet запускается обычной командой `make cabinet`. Создайте новую заявку в кабинете, затем проверьте данные:

```sh
docker compose --env-file .env -f deploy/compose/compose.yaml exec postgres \
  psql -U search_owner -d search -c \
  "SELECT request_id,status,created_at,cancelled_at,received_at FROM search_requests ORDER BY received_at DESC LIMIT 20"

docker compose --env-file .env -f deploy/compose/compose.yaml exec postgres \
  psql -U search_owner -d search -c "SELECT count(*) FROM inbox_events"
```

После отмены заявки состояние в Search становится `cancelled`. В Cabinet сохранённая заявка остаётся `saved`, пока следующий этап не начнёт фактическую работу.

## Конфигурация

[services/search/config.yaml](../../services/search/config.yaml) обязателен. Путь задаёт `SEARCH_CONFIG`. Приоритет: env процесса → корневой `.env` → YAML. Viper проверяет неизвестные ключи и типы; все поля переопределяются через префикс `SEARCH_`, например `SEARCH_CONSUMER_GROUP_ID`, `SEARCH_CONSUMER_BROKERS`, `SEARCH_DATABASE_PORT`.

`migrate` использует только пароль владельца, `consume` — приложения. Брокеры задаются списком YAML или строкой env с запятыми. `consumer` содержит topic, group_id, таймауты БД, подключения и подтверждения Kafka. Локальные Kafka-соединения без TLS; публичное развёртывание пока не подготовлено.

## Stage_local и dev

Добавьте два пароля Search в соответствующий `.env.stage_local`/`.env.dev`. `make stage-local-up` собирает Search, `make dev-up` скачивает его образ с тем же IMAGE_TAG, что Cabinet и frontend.

Порядок зависимостей: PostgreSQL → `search-db-init` → `search-migrate` → `search`; потребитель также ждёт `kafka-init`. Одноразовые контейнеры штатно завершаются с кодом 0. Отдельного HTTP endpoint у Search нет; `up --wait` подтверждает запуск процесса, а получение события проверяется запросом к БД и логами `search`.

`search-db-init` добавляет роли и базу на существующем томе без удаления данных Cabinet. Повторный запуск не меняет пароли уже существующих ролей. Для смены пароля нужна отдельная операция PostgreSQL. Системный пароль PostgreSQL передаётся только одноразовому контейнеру инициализации, не приложению Search.

Новый Compose dev требует публикации образа Search; старый dev-latest до этого МР его не содержит. Не запускайте обновлённый dev до успешной публикации всех трёх образов.

Для остановки только потребителя:

```sh
docker compose --env-file .env.stage_local -f deploy/stage_local/compose.yaml stop search
```

Остановка не удаляет БД и offsets. Откат образа не откатывает миграции. Не удаляйте inbox или записи отмены для повторной обработки.

## Проверки

`make check` включает unit-тесты конфигурации, разбора событий и порядка подтверждения. `make test-int` проверяет миграции и транзакции Search в изолированной схеме своей БД; предварительно нужен `make search-db-init`.

`make test-kafka` последовательно проверяет Cabinet и Search на отдельном тестовом брокере. Search использует случайные топик, группу и схему; проверяет потерю подтверждения после записи БД, повтор после перезапуска, дубли, позднее создание после отмены и сохранённые offsets. Тесты не скачивают справочник и не используют поставщиков билетов.
