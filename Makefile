.DEFAULT_GOAL := db-up

GOLANGCI_LINT_VERSION = v2.8.0
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: lint gosec
lint:
	$(GOLANGCI_LINT) run --config .golangci.yml ./...

gosec:
	$(GOLANGCI_LINT) run --config .golangci.yml --enable-only gosec ./...

COMPOSE = docker compose --env-file .env -f deploy/compose/compose.yaml

.PHONY: db-up db-stop migrate cabinet frontend generate test test-int check

db-up:
	$(COMPOSE) up -d --wait postgres

db-stop:
	$(COMPOSE) stop postgres

migrate:
	CABINET_CONFIG=$${CABINET_CONFIG:-services/cabinet/config.yaml} go run ./services/cabinet/cmd/cabinet migrate

cabinet:
	CABINET_CONFIG=$${CABINET_CONFIG:-services/cabinet/config.yaml} go run ./services/cabinet/cmd/cabinet serve

frontend:
	npm --prefix frontend run dev

generate:
	buf lint
	buf generate

test:
	go test -race ./...

test-int:
	go test -race -tags=integration ./services/cabinet/internal/auth ./services/search/internal/storage -count=1

check: lint
	buf lint
	go test -race ./...
	npm --prefix frontend run build

STAGE_LOCAL = docker compose --env-file .env.stage_local -f deploy/stage_local/compose.yaml
.PHONY: stage-local-up stage-local-stop stage-local-restart stage-local-logs stage-local-cities-sync

stage-local-cities-sync:
	$(STAGE_LOCAL) run --rm --build cities-sync

stage-local-up:
	$(STAGE_LOCAL) up --build -d --wait --wait-timeout 120

stage-local-stop:
	$(STAGE_LOCAL) stop

stage-local-restart:
	$(STAGE_LOCAL) restart postgres kafka outbox search cabinet frontend

stage-local-logs:
	$(STAGE_LOCAL) logs --tail 100 -f cabinet frontend outbox search kafka

DEV = docker compose --env-file .env.dev -f deploy/dev/compose.yaml
.PHONY: dev-up dev-stop dev-restart dev-logs

dev-up:
	$(DEV) pull
	$(DEV) up --no-build --pull never -d --wait --wait-timeout 120

dev-stop:
	$(DEV) stop

dev-restart:
	$(DEV) restart postgres kafka outbox search cabinet frontend

dev-logs:
	$(DEV) logs --tail 100 -f migrate search-migrate cabinet frontend outbox search kafka

.PHONY: cities-sync
cities-sync:
	CABINET_CONFIG=$${CABINET_CONFIG:-services/cabinet/config.yaml} go run ./services/cabinet/cmd/cabinet sync-cities

.PHONY: publish-outbox
publish-outbox:
	CABINET_CONFIG=$${CABINET_CONFIG:-services/cabinet/config.yaml} go run ./services/cabinet/cmd/cabinet publish-outbox

.PHONY: kafka-up kafka-stop test-kafka
kafka-up:
	$(COMPOSE) up -d --wait --wait-timeout 180 kafka
	$(COMPOSE) run --rm kafka-init

kafka-stop:
	$(COMPOSE) stop kafka

test-kafka:
	docker compose -p travel-watch-kafka-test -f deploy/test/compose.yaml up -d --wait --wait-timeout 180 kafka
	go test -race -tags=integration,kafka ./services/cabinet/internal/auth -run '^TestKafkaOutbox$$' -count=1 -timeout=3m
	go test -race -tags=integration,kafka ./services/search/internal/storage -run '^TestKafkaInbox$$' -count=1 -timeout=2m

.PHONY: search-db-init search-migrate search
search-db-init:
	$(COMPOSE) run --rm search-db-init

search-migrate:
	SEARCH_CONFIG=$${SEARCH_CONFIG:-services/search/config.yaml} go run ./services/search/cmd/search migrate

search:
	SEARCH_CONFIG=$${SEARCH_CONFIG:-services/search/config.yaml} go run ./services/search/cmd/search consume
