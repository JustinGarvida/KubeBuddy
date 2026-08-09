# Design: docker-compose for RabbitMQ and TimescaleDB

**Issue**: [#10](https://github.com/JustinGarvida/PodSentinel/issues/10)
**Branch**: `10/build/compose-rabbitmq-timescaledb`
**Date**: 2026-08-09

## Problem

PodSentinel's architecture (`docs/architecture.md`) depends on RabbitMQ (event bus between Go and Python) and Postgres/TimescaleDB (source of truth for metrics/anomalies), but no local instance of either exists yet. The Go agent's Postgres writer and RabbitMQ publisher, and the Python anomaly detector, all need something to talk to before they can be built. `docs/architecture.md`'s testing strategy already names docker-compose as the mechanism for bringing up RabbitMQ + Postgres for integration testing — this design implements that piece.

## Why not Kubernetes (KIND) for these services?

Considered and rejected for now: running RabbitMQ/TimescaleDB inside the existing KIND cluster (`infra/kind/`) instead of docker-compose.

The only component that actually needs to run *inside* Kubernetes is the Go agent itself, because it needs in-cluster RBAC access to `metrics.k8s.io` via `client-go`. RabbitMQ and TimescaleDB are plain network services the agent talks to over configured host/port — they're indifferent to whether they run in Docker or in a cluster. Hosting them in KIND now would mean taking on StatefulSets/PVCs/storage classes and Helm charts (or hand-rolled manifests) for two stateful services, before any code exists that writes to either — complexity the project's "prove the simple thing first" build order doesn't call for yet.

This does mean a split-brain local dev setup once the Go agent is later run inside KIND: the agent would reach docker-compose's services via `host.docker.internal` (works out of the box on macOS with Docker Desktop, which is what KIND runs on). That tradeoff is accepted for now. Moving RabbitMQ/TimescaleDB into the cluster is a reasonable future step once the agent is actually deployed in-cluster and a single unified environment is worth the added complexity — not a blocker for this change.

## Scope

### `infra/docker-compose.yml`

Two services:

- **`rabbitmq`** — `rabbitmq:management` image (includes the management UI plugin). Ports: `5672` (AMQP), `15672` (management UI). Credentials via `RABBITMQ_DEFAULT_USER` / `RABBITMQ_DEFAULT_PASS` env vars. Named volume for persistence.
- **`timescaledb`** — `timescale/timescaledb:latest-pg16` image. Port `5432`. Credentials via `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` env vars. Named volume for persistence.

Both services read credentials from `infra/.env` (via `env_file:`), not hardcoded into the compose file.

No init SQL, no schema creation (`pod_metrics` hypertable, `anomalies` table) — deferred until the Go agent has actual DB-writing code, so the schema isn't defined speculatively ahead of what the code needs.

No RabbitMQ topology (exchanges, queues, DLQs) predefined via `definitions.json` — deferred until Go/Python pub/sub code exists to consume it, for the same reason.

### `infra/.env.example` + `infra/.gitignore`

`infra/.env.example` — checked in, dev-only default credentials (e.g. `postgres`/`postgres`, `guest`/`guest`), following the same pattern as `go/.env.example`. `infra/.gitignore` containing `.env` so the real file (copied from the example) is never committed — mirrors `go/.gitignore`'s handling of `go/.env`.

### `docs/infra/docker-compose.md`

Mirrors the structure of `docs/infra/kind.md`: prerequisites (Docker), bring-up (`docker compose -f infra/docker-compose.yml up -d`), verification steps for each service, teardown, and the connection details (host/port/credentials) that the Go agent's config and the Python detector's config will need later. Cross-links to `docs/architecture.md`.

## Out of scope

- Kubernetes/KIND integration for these services (see rejected-approach discussion above).
- Pre-created Postgres schema.
- Pre-declared RabbitMQ exchanges/queues/DLQs.
- Wiring the Go agent or Python detector to actually connect to these services — that's separate, later work once this infra exists.

## Testing / Verification

- `docker compose -f infra/docker-compose.yml up -d` brings up both services cleanly.
- RabbitMQ management UI reachable at `http://localhost:15672` with the configured credentials.
- `psql`/`pg_isready` (or equivalent) confirms TimescaleDB is accepting connections on `5432` with the configured credentials.
- `docker compose down` / `down -v` correctly tear down (with `-v` removing volumes, without it preserving them).
