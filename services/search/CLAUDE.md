# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Scope: `services/search` only. See the root `CLAUDE.md` for repo-wide commands/architecture and `AGENTS.md` for working conventions. Service status and responsibilities: `docs/services/search.md`.

Search consumes trip-request events from Cabinet, builds composite route candidates, and (as of the in-progress work) checks saved schemas against real timetables. Pricing, availability, periodic re-checks and provider rate limiting are not implemented yet — check `docs/services/search.md`'s status line before assuming a capability exists.

## Package layout

`cmd/search/main.go` dispatches on `os.Args[1]` (default `consume`): `migrate`, `consume` (Kafka inbox, the normal long-running process), `build-routes`, `publish-progress`, `serve-results`, `plan-route`, `airports-sync`, `airports-find`, `compare-planners`, `compare-real-routes`.

- `internal/consumer` — Kafka inbox consumer for trip-request events ([ADR 0006](../../docs/adr/0006-search-inbox.md)).
- `internal/storage` — queries/transactions for Search's own Postgres schema, mostly static SQL text (leases, `SKIP LOCKED`, `ON CONFLICT`) per the ADR 0002 amendment (results, schedule checks, etc.).
- `internal/config` — Viper YAML + env loading (`services/search/config.yaml`, override path via `SEARCH_CONFIG`).
- `internal/routes` — `GraphPlanner`: the own-algorithm route-candidate builder, originally on a synthetic dataset.
- `internal/realroutes` — builds schemas from real Collector responses (via `transport-stdio`): picks candidate airports/stations, checks links, tags assumed transfers. Backs both `plan-route` and the automatic `build-routes` graph path.
- `internal/planning` — the `Planner` interface and `Compare` (shared validation across planners, [ADR 0007](../../docs/adr/0007-route-planners.md)).
- `internal/gemini` — the Gemini-backed planner; runs independently of GraphPlanner, never receives its candidates ([ADR 0011](../../docs/adr/0011-independent-planner-runs.md)).
- `internal/journeys` — synthetic-schedule journey assembly + time-compatibility checks ([spec](../../docs/specs/scheduled-journeys.md)); separate from `internal/schedules`, not yet wired to real events.
- `internal/schedules` — **in-progress**: checks saved `realroutes` schemas against real Yandex timetables for specific dates, stores `schedule_checks`, feeds the Cabinet "Стыковки" tab ([ADR 0013](../../docs/adr/0013-timetable-checks.md), [docs/ops/schedules.md](../../docs/ops/schedules.md)). Uses leased jobs with a bounded retry count, cancellation checks before/after each Collector call, and rejects stale writes by comparing lease tokens.
- `internal/airports` — OurAirports catalog + manual sync/lookup commands ([docs/ops/airport-catalog.md](../../docs/ops/airport-catalog.md)).
- `internal/results` — `serve-results` gRPC/mTLS server exposing saved schemas to Cabinet ([ADR 0012](../../docs/adr/0012-saved-routes-grpc.md)).
- `internal/routeexperiment`, `internal/evaluation` — research/comparison tooling behind `compare-planners`/`compare-real-routes`, not part of the production path.

## Commands

```sh
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search migrate
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search consume        # Kafka inbox
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search build-routes   # route building + schedule-check loops
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search serve-results   # gRPC/mTLS results server
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search publish-progress
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search airports-sync
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search airports-find "$IATA"
SEARCH_CONFIG=services/search/config.yaml go run ./services/search/cmd/search plan-route -from "..." -to "..." -from-lat .. -from-lon .. -to-lat .. -to-lon ..
```

Equivalent Make targets: `make search-migrate`, `make search`, `make search-build-routes`, `make search-results`, `make search-publish-progress`, `make search-airports-sync`, `make search-airports-find IATA=...`, `make search-plan-route FROM=... TO=... ...`, `make search-compare`, `make search-compare-real`.

`build-routes`, `plan-route` and `search-compare-real` all shell out to a locally built Collector binary over the `api/transport` stdio protocol — `make search-build-routes` / `make search-plan-route` build `bin/collector` first (`go build -o bin/collector ./services/collector/cmd/collector`); running the underlying `go run` command directly requires that binary to already exist at `bin/collector`.

`serve-results` additionally loads `.local/tls/search.env` if present (server mTLS cert env); generate with `make tls-init`.

## Tests

- `go test -race ./services/search/...` — most packages need no DB (e.g. `routes`, `journeys`, `planning`, `schedules`'s pure-logic tests: night rollover, timezone offsets, exact-margin boundary, unknown transfers, cancellation, call limits, dedup of identical calls within one run).
- `make test-int` runs `go test -race -tags=integration ./services/search/internal/storage ...` — needs local Postgres, exercises save/restore, lease recovery, late writes, cancellation for `schedule_checks` and results storage.
- `make test-kafka` runs `TestKafkaInbox` against a throwaway Kafka broker.
- `schedules.spec.ts` / `route-results.spec.ts` under `frontend/e2e` cover the Cabinet-side rendering against mocked API responses.

## Notes specific to this service

- `internal/schedules` config lives under the `schedules:` key in `services/search/config.yaml` — transfer durations, margins, per-run budgets (external calls, days, itinerary/combination limits). See `docs/ops/schedules.md` for the exact defaults and their status as policy, not carrier guarantees.
- `schedules.enabled: true` also picks up previously saved active requests with a graph result. Active requests are re-checked every `schedules.recheck_interval` (1h; 0 disables); a failed re-check keeps the previous result. Cancelled and expired requests are not re-checked. There is no manual retry yet.
- Only run one `route-builder` process; multiple instances split inbox work but do not share a request budget against Yandex/Google Flights.
- Applied migrations are never edited — add a new file under `migrations/`.
