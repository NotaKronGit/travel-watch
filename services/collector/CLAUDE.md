# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Scope: `services/collector` only. See the root `CLAUDE.md` for repo-wide commands/architecture and `AGENTS.md` for working conventions. Service status and responsibilities: `docs/services/collector.md`.

Collector normalizes calls to transport providers. **It is not a persistent service yet** — no RabbitMQ job consumer, no Kafka publisher, no own database. It is invoked either as a one-off CLI command or, more commonly, as a subprocess Search shells out to (`transport-stdio`) over the JSON protocol defined in `api/transport/protocol.go`. Don't assume the broker-based architecture in `docs/architecture.md` exists here yet — it's the target, not the current state.

## Package layout

`cmd/collector/main.go` dispatches on `os.Args[1]`: `stations-find` (flags: `-lat`, `-lon`, `-radius` default 20, `-limit` default 20), `flights-find`, `transport-stdio` (no flags — serves sequential JSON requests on stdin/stdout for a caller like Search).

- `internal/rail` — two independent interfaces a provider may implement one or both of: `StationProvider.FindStations` (text+country or coordinates+radius mode, radius capped at 50km) and `TrainProvider.FindTrains` (two stations of the *same* provider, local date, 1–9 adults, limit 1–100). These are internal Go contracts — Search does not import this package directly; it talks over `api/transport`.
  - `internal/rail/yandex` — the real adapter (Yandex Schedules API). Requires `yandex.api_key` in config.
  - `SeatProvider.FindSeats` (same-provider stations, train number, local date → cars with normalized class and free seats with number, position and one-adult price). `ErrTrainNotFound` and `ErrNotOnSale` are distinct from errors and from a complete result with no free seats; adapters must run `SeatResult.Validate` before returning. No real seat adapter yet — see `docs/specs/seat-availability.md`.
  - `internal/rail/testprovider` — synthetic, three fictional stations in two cities, fixed 2027-01-10 offers, no I/O, used in tests and as the default local experiment provider.
- `internal/flights` — `Provider` interface for flight search by airports/dates/adults.
  - `tools/fli` — a Python bridge (`bridge.py`) that the Fli flight-search implementation shells out to as an isolated local subprocess; requires `make flights-setup` (creates `.local/fli-venv`) before use.
- `api/transport/protocol.go` (repo root, not under this service) — the stdio JSON protocol: locate a place by coordinates, nearby stations, direct trains/flights between two points, arrivals, flight stops. Connections only — no prices, no seats, no time-compatibility checking; that stays in Search.

All provider results carry: source, `Synthetic`, observed-at timestamp, `Complete`, `LimitReached`. A non-nil error means the result must not be used; a successful-empty result is distinct from `ErrUnavailable`/`ErrRateLimited`/`ErrUnsupported`/context cancellation — never collapse an error into a fake empty success.

## Commands

```sh
go run ./services/collector/cmd/collector stations-find -lat 55.75 -lon 37.62 -radius 20 -limit 20
go run ./services/collector/cmd/collector flights-find <args>
go run ./services/collector/cmd/collector transport-stdio   # normally launched by Search, not run manually
```

Make targets: `make collector-stations-find LAT=.. LON=.. [RADIUS=20] [LIMIT=20]`, `make flights-setup` (Python venv, needs `python3` >=3.10), `make flights-find ARGS="..."`.

Config: `services/collector/config.yaml` (override path via `COLLECTOR_CONFIG`), Viper YAML + env, no database. Holds `yandex.api_key`/timeout/max-response-bytes and the `fli` Python bridge path/timeout.

## Tests

```sh
go test -race ./services/collector/...
```

No DB or broker needed. `internal/rail/testprovider` and adapter unit tests cover empty results, truncation, ID integrity, group pricing, data isolation between calls, errors, cancellation and deadline expiry. `services/collector/internal/rail/yandex/schedules_test.go` is new/in-progress work backing the Search `internal/schedules` timetable-check feature.

## Notes specific to this service

- The Yandex API key is a secret — never commit a real value into `config.yaml`; keep placeholders as shipped.
- All provider methods must respect the caller's `context` deadline/cancellation; real adapters additionally bound their own HTTP timeout and response size.
- Architecture background: [ADR 0008](../../docs/adr/0008-rail-provider-lookups.md) (why stations are looked up online instead of a bulk world import), [ADR 0009](../../docs/adr/0009-manual-real-route-planner.md) (the `transport-stdio` experiment's boundaries). Station search: [`docs/ops/rail-stations.md`](../../docs/ops/rail-stations.md). Flight search: [`docs/ops/flight-search.md`](../../docs/ops/flight-search.md).
