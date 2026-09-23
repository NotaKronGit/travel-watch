# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Scope: `services/notification`. **This service is not implemented.** `cmd/`, `internal/` and `migrations/` currently contain only `.gitkeep` placeholders — there is no code, no config, and nothing to build, run, or test here yet.

Design intent (from `docs/services/notification.md`, not yet built):

- Owns Telegram bindings and notification-delivery state. Trip requests remain owned by Cabinet, search results by Search — Notification does not run its own searches.
- Binding starts in the authenticated web Cabinet with a short-lived one-time code; contracts between Cabinet and Notification for this are not defined yet.
- Consumes notification events from Search over Kafka; bot commands (list requests, request card, pause/resume, link to site) are inbound Telegram API traffic.
- Delivery must be idempotent with bounded retries/backoff; exactly-once delivery to Telegram is explicitly not assumed.

## Before writing code here

Per `AGENTS.md`, read `docs/product.md` (Telegram section), `docs/architecture.md`, `docs/roadmap.md` and `docs/services/notification.md` first, and raise a new ADR under `docs/adr/` for any binding/delivery decision — there isn't one yet for this service. Do not pre-create database tables, config scaffolding, or dependencies ahead of an actual implementation step; the repo's convention (see `docs/adr/0002-service-storage-and-configuration.md` and the `services/cabinet`/`services/search` implementations) is `internal/storage` with goqu + database/sql + Goose migrations and a service-owned YAML config with env overrides — follow that shape once this service is actually built, rather than building ahead of the roadmap.
