# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Scope: `services/cabinet` only. See the root `CLAUDE.md` for repo-wide commands/architecture and `AGENTS.md` for working conventions. Service status and responsibilities: `docs/services/cabinet.md`.

Cabinet owns users, sessions and trip requests, and is the HTTP/ConnectRPC API the frontend talks to. It does not own search results, price history, or route candidates — those belong to Search and are read through `internal/searchclient` (gRPC/mTLS).

## Package layout

- `cmd/cabinet/main.go` — single entrypoint, dispatches on `os.Args[1]`: `serve` (default), `migrate`, `sync-cities`, `publish-outbox`, `consume-progress`.
- `internal/auth` — HTTP/RPC handlers, session and registration logic (this is also where the ConnectRPC handler tree is wired via `auth.Handler(...)`).
- `internal/trips` — trip-request RPC handlers (`trips.Handler(...)`), composed into the same router as auth.
- `internal/storage` — all queries (goqu, plus SQL text for the outbox lease and row locks) and transactions against Cabinet's own Postgres schema; registration+session creation and outbox writes are atomic here.
- `internal/config` — Viper YAML + env loading (`services/cabinet/config.yaml`, override path via `CABINET_CONFIG`).
- `internal/outbox` — transactional outbox relay (`publish-outbox` command) that ships trip-request events to Kafka at-least-once ([ADR 0004](../../docs/adr/0004-trip-request-outbox.md), [docs/ops/outbox.md](../../docs/ops/outbox.md)).
- `internal/catalog` — GeoNames city-catalog import (`sync-cities` command) via a `catalog.Source` interface; test source has no network calls.
- `internal/progress` — Kafka consumer (`consume-progress` command) for Search's route-building progress/history events (v1 and v2 payloads), with dedup/out-of-order/resume protection.
- `internal/searchclient` — gRPC/mTLS client used by `trips` to fetch saved route schemas and progress from Search ([ADR 0012](../../docs/adr/0012-saved-routes-grpc.md)).

## Commands

```sh
CABINET_CONFIG=services/cabinet/config.yaml go run ./services/cabinet/cmd/cabinet serve
CABINET_CONFIG=services/cabinet/config.yaml go run ./services/cabinet/cmd/cabinet migrate
CABINET_CONFIG=services/cabinet/config.yaml go run ./services/cabinet/cmd/cabinet sync-cities
CABINET_CONFIG=services/cabinet/config.yaml go run ./services/cabinet/cmd/cabinet publish-outbox
CABINET_CONFIG=services/cabinet/config.yaml go run ./services/cabinet/cmd/cabinet consume-progress
```

Equivalent Make targets from the repo root: `make cabinet`, `make migrate`, `make cities-sync`, `make publish-outbox`, `make cabinet-consume-progress`.

`serve` additionally loads `.local/tls/cabinet.env` if present (local mTLS client cert env for `searchclient`); generate dev certs with `make tls-init` first if it's missing.

## Tests

- `go test -race ./services/cabinet/...` — no DB required for plain unit tests.
- `make test-int` runs `go test -race -tags=integration ./services/cabinet/internal/auth ...` — needs a local Postgres (`make db-up`), creates and drops a random schema per run.
- `make test-kafka` runs the `TestKafkaOutbox` integration test against a throwaway Kafka broker (`deploy/test/compose.yaml`).
- Browser/e2e tests live in `frontend/e2e` and exercise Cabinet's real HTTP API (see root `CLAUDE.md` for the frontend test commands); they are not part of this package.

## Notes specific to this service

- Registration requires a 15+ character password; `.env` must set five distinct local Postgres passwords (see `docs/ops/local-development.md`).
- `CABINET_AUTH_COOKIE_SECURE=false` is only valid for local HTTP; production cookie/session config is not defined yet.
- Applied migrations are never edited — add a new SQL file under `migrations/`.
- Spec references: [`docs/specs/cabinet-auth.md`](../../docs/specs/cabinet-auth.md), [`docs/specs/trip-request.md`](../../docs/specs/trip-request.md), city catalog: [`docs/ops/city-catalog.md`](../../docs/ops/city-catalog.md).
