# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Travel Watch is a learning project (Go backend + React/TypeScript frontend) for finding composite trips (flight/rail combinations) and tracking their price. See `README.md` for the product pitch and current status.

**Read `AGENTS.md` first.** It is the authoritative, checked-in working agreement for this repo (language, how to sequence work, which docs to read before changing a given area, ADR/PR conventions, repo/branch rules). This file does not repeat that agreement — it is the engineering map: commands and architecture. Each service also has its own `services/<name>/CLAUDE.md` with service-specific commands and package layout.

## Monorepo layout

Single `go.mod` at the root, one binary per service, one PostgreSQL database per service (no service reads another's tables):

```
services/{cabinet,search,collector,notification}/{cmd,internal,migrations}/
frontend/            React Admin + TypeScript (Vite)
api/proto/           Protobuf contracts (source of truth)
gen/                 Generated Go from api/proto (checked in)
frontend/src/gen/    Generated TypeScript from api/proto (checked in)
api/transport/       JSON stdio protocol between Search and the Collector binary (not gRPC)
internal/platform/   Shared *technical* Go code only — no business logic
internal/devpki/     Local mTLS cert generation for dev (tools/devcerts)
deploy/              docker-compose files: compose (base db/kafka), dev, stage_local, test
docs/                Product, architecture, roadmap, per-service docs, ADRs, ops runbooks
```

Regenerating `gen/` and `frontend/src/gen/` from `api/proto/` requires `make generate` (needs Buf CLI + frontend deps installed first). Generated files are committed and are never hand-edited.

## Common commands

Run from the repo root unless noted. Full walkthroughs: `docs/ops/local-development.md`, `docs/ops/stage-local.md`, `docs/ops/dev-images.md`.

```sh
make db-up                 # start Postgres only (docker compose)
make migrate               # Cabinet migrations (Goose, owner role)
make search-migrate        # Search migrations
make cabinet                # go run Cabinet (serve)
make frontend                # npm run dev (Vite, localhost:5173)
make search                  # go run Search (consume, Kafka inbox)
make search-build-routes     # Search route-building + schedule-check loop (builds bin/collector first)
make search-results          # Search gRPC/mTLS results server
```

Checks:

```sh
make check          # golangci-lint (incl. gosec/govet), buf lint, go test -race ./..., frontend build+typecheck
make test           # go test -race ./... only
make test-int       # integration tests, needs local Postgres (services/cabinet/internal/auth, services/search/internal/storage)
make test-kafka     # Kafka-tagged tests against a throwaway broker (deploy/test/compose.yaml)
make lint           # golangci-lint only
npm --prefix frontend test      # vitest unit tests
npm --prefix frontend run test:e2e   # Playwright, needs Cabinet running
```

A single Go test: `go test -race ./services/search/internal/schedules/... -run TestName`. Integration-tagged tests need `-tags=integration` (and `,kafka` for the Kafka ones) — see the `test-int`/`test-kafka` Makefile targets for the exact invocations, they are not just `go test ./...`.

`make check` is what CI (`docs/ops/ci.md`) treats as the baseline; CI additionally runs `test-int`, `test-kafka`, `test-progress-e2e` and browser tests against a real Postgres/Kafka, and builds all three Docker images. A green `make check` locally does not confirm the GitHub Actions run or image publish.

## Architecture — data flow

1. **Cabinet** saves a trip request and publishes its change to Kafka via a transactional outbox ([ADR 0004](docs/adr/0004-trip-request-outbox.md)).
2. **Search** consumes it through a transactional inbox ([ADR 0006](docs/adr/0006-search-inbox.md)), dedupes identical searches, and schedules checks.
3. Search hands work to **Collector** — currently via a local JSON stdio protocol (`api/transport`, Search shells out to a built `bin/collector transport-stdio`), not yet via RabbitMQ.
4. Collector adapters (currently Yandex Schedules for rail, Fli for flights) return normalized observations.
5. Search builds routes (two independent planners — see below), stores results, and (for saved graph schemas) checks timetable feasibility.
6. **Notification** (not yet implemented) is meant to deliver Telegram alerts from Search's notification events.

The target architecture (RabbitMQ for Collector jobs, Kafka for facts/results, Redis for shared provider rate limits) is documented in `docs/architecture.md` — most of Collector's broker integration and Notification itself do not exist yet; do not assume they do. Check each service's status line in `docs/services/*.md` before relying on a capability.

## Storage & config pattern (ADR 0002)

Cabinet and Search both follow the same shape — reuse it, don't introduce a different one without discussing the ADR change:

- Own `internal/storage` package per service, `database/sql` (pgx driver) — no ORM. `goqu` for queries built programmatically and simple CRUD; static SQL text for lock/lease/`ON CONFLICT` queries, in the format fixed by the ADR 0002 amendment.
- SQL migrations via Goose, embedded in the binary from `services/<name>/migrations`, run with `<binary> migrate`.
- Config: `services/<name>/config.yaml` loaded through Viper, overridable by env vars (see `docs/ops/configuration.md` for the naming convention and precedence). Override the config path via `<SERVICE>_CONFIG` env var (e.g. `SEARCH_CONFIG`).

Collector currently has a config.yaml (provider API keys/timeouts) but no database — it's CLI-invoked, not a persistent process yet.

## Two independent route planners

Search builds route candidates with two independent implementations behind a shared `Planner` interface and a common validator ([ADR 0007](docs/adr/0007-route-planners.md)): `GraphPlanner` (own algorithm, `internal/routes`, `internal/realroutes`) and a Gemini-backed planner (`internal/gemini`). They run independently per trip request and must not share intermediate state or influence each other's output ([ADR 0011](docs/adr/0011-independent-planner-runs.md)). `make search-compare` / `make search-compare-real` run them side by side for research, outside the normal consumer path.

## Concurrency and correctness conventions

- Go code uses `context` end-to-end, explicit timeouts on external calls, bounded worker pools/goroutine counts (never unbounded goroutines per job), and graceful shutdown on `SIGTERM`/`SIGINT`.
- Message handlers must tolerate redelivery, partial failure, and out-of-order/late results — this is enforced by dedicated tests (leases, cancellation checks, idempotent writes), not just by convention. Follow the existing pattern (e.g. `services/search/internal/schedules`, `services/cabinet/internal/outbox`) rather than reinventing it.
- Provider errors are never turned into a successful-empty result; incompleteness (rate limits, pagination limits, unsupported queries) is represented explicitly and propagated, not swallowed.

## Key docs to check before changing an area

- `docs/product.md` — MVP scope and data-correctness rules.
- `docs/architecture.md` — target architecture and current status per area (has drifted ahead of `main`; see `git status` for in-flight work).
- `docs/roadmap.md` — sequencing and open questions.
- `docs/services/<cabinet|search|collector|notification>.md` — per-service status and responsibilities.
- `docs/adr/` — accepted architectural decisions.
