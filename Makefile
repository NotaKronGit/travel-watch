COMPOSE = docker compose --env-file .env -f deploy/compose/compose.yaml

.PHONY: db-up db-stop migrate cabinet frontend generate test test-int check

db-up:
	$(COMPOSE) up -d --wait postgres

db-stop:
	$(COMPOSE) stop postgres

migrate:
	go run ./services/cabinet/cmd/cabinet migrate

cabinet:
	go run ./services/cabinet/cmd/cabinet serve

frontend:
	npm --prefix frontend run dev

generate:
	buf lint
	buf generate

test:
	go test -race ./...

test-int:
	go test -race -tags=integration ./services/cabinet/internal/auth -count=1

check:
	buf lint
	go vet ./...
	go test -race ./...
	npm --prefix frontend run build

STAGE_LOCAL = docker compose --env-file .env.stage_local -f deploy/stage_local/compose.yaml
.PHONY: stage-local-up stage-local-stop stage-local-restart stage-local-logs

stage-local-up:
	$(STAGE_LOCAL) up --build -d --wait --wait-timeout 120

stage-local-stop:
	$(STAGE_LOCAL) stop

stage-local-restart:
	$(STAGE_LOCAL) restart postgres cabinet frontend

stage-local-logs:
	$(STAGE_LOCAL) logs --tail 100 -f cabinet frontend

DEV = docker compose --env-file .env.dev -f deploy/dev/compose.yaml
.PHONY: dev-up dev-stop dev-restart dev-logs

dev-up:
	$(DEV) pull
	$(DEV) up --no-build --pull never -d --wait --wait-timeout 120

dev-stop:
	$(DEV) stop

dev-restart:
	$(DEV) restart postgres cabinet frontend

dev-logs:
	$(DEV) logs --tail 100 -f migrate cabinet frontend
