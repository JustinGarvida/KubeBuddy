# Compose RabbitMQ + TimescaleDB Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give local development a working RabbitMQ instance and a TimescaleDB (Postgres) instance via docker-compose, per the integration testing strategy in `docs/architecture.md`.

**Architecture:** A single `infra/docker-compose.yml` defines two independent services — `rabbitmq` (with the management UI plugin) and `timescaledb` (Postgres + TimescaleDB extension) — each with a named volume for persistence and credentials sourced from a gitignored `infra/.env` file. No app code, no DB schema, no RabbitMQ topology — this is backing-service infra only.

**Tech Stack:** Docker Compose (v2 CLI, `docker compose`), `rabbitmq:management` image, `timescale/timescaledb:latest-pg16` image.

## Global Constraints

- No Kubernetes/KIND integration for these services — they run on the host via docker-compose (see spec's "Why not Kubernetes" section).
- No Postgres schema (`pod_metrics`, `anomalies`) — deferred until the Go agent has DB-writing code.
- No RabbitMQ exchange/queue/DLQ topology — deferred until Go/Python pub/sub code exists.
- Credentials live only in `infra/.env` (gitignored), never hardcoded into `infra/docker-compose.yml`. `infra/.env.example` holds the checked-in dev defaults.
- Follow the existing repo pattern from `go/.env.example` / `go/.gitignore` (see `docs/superpowers/specs/2026-08-09-compose-rabbitmq-timescaledb-design.md`).
- Conventional Commits format for every commit (type `build` or `docs`, scope `infra`).

---

### Task 1: Env file scaffolding

**Files:**
- Create: `infra/.gitignore`
- Create: `infra/.env.example`

**Interfaces:**
- Produces: `infra/.env.example` — the variable names `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS`, which Task 2 and Task 3's `docker-compose.yml` reference via `env_file:`.

- [ ] **Step 1: Create `infra/.gitignore`**

```
.env
```

- [ ] **Step 2: Create `infra/.env.example`**

```
# Copy to infra/.env and adjust as needed for local development.
# Referenced by infra/docker-compose.yml via env_file: this file is not
# auto-copied — `cp infra/.env.example infra/.env` before running compose.

# --- TimescaleDB / Postgres ---
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=podsentinel

# --- RabbitMQ ---
RABBITMQ_DEFAULT_USER=guest
RABBITMQ_DEFAULT_PASS=guest
```

- [ ] **Step 3: Verify the ignore rule works**

Run:
```bash
cp infra/.env.example infra/.env
git status --short infra/
```
Expected: only `infra/.gitignore` and `infra/.env.example` show as untracked (`??`) — `infra/.env` must NOT appear. Then clean up:
```bash
rm infra/.env
```

- [ ] **Step 4: Commit**

```bash
git add infra/.gitignore infra/.env.example
git commit -m "build(infra): add env file scaffolding for compose credentials"
```

---

### Task 2: TimescaleDB service

**Files:**
- Create: `infra/docker-compose.yml`

**Interfaces:**
- Consumes: `infra/.env` variables `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` (Task 1).
- Produces: a `timescaledb` service reachable at `localhost:5432`, which Task 3 adds a sibling service alongside, and which `docs/infra/docker-compose.md` (Task 4) documents.

- [ ] **Step 1: Create `infra/docker-compose.yml` with the `timescaledb` service**

```yaml
services:
  timescaledb:
    image: timescale/timescaledb:latest-pg16
    container_name: podsentinel-timescaledb
    env_file: .env
    ports:
      - "5432:5432"
    volumes:
      - timescaledb-data:/var/lib/postgresql/data

volumes:
  timescaledb-data:
```

- [ ] **Step 2: Verify config parses**

Run:
```bash
cp infra/.env.example infra/.env
docker compose -f infra/docker-compose.yml config --quiet
```
Expected: no output, exit code 0 (confirms valid YAML and resolvable env references).

- [ ] **Step 3: Bring the service up and verify it accepts connections**

Run:
```bash
docker compose -f infra/docker-compose.yml up -d timescaledb
docker compose -f infra/docker-compose.yml exec timescaledb pg_isready -U postgres
```
Expected: `pg_isready` reports `accepting connections`.

- [ ] **Step 4: Tear down**

Run:
```bash
docker compose -f infra/docker-compose.yml down -v
rm infra/.env
```

- [ ] **Step 5: Commit**

```bash
git add infra/docker-compose.yml
git commit -m "build(infra): add TimescaleDB service to docker-compose"
```

---

### Task 3: RabbitMQ service

**Files:**
- Modify: `infra/docker-compose.yml` (add `rabbitmq` service and its volume alongside `timescaledb`)

**Interfaces:**
- Consumes: `infra/.env` variables `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS` (Task 1).
- Produces: a `rabbitmq` service reachable at `localhost:5672` (AMQP) and `localhost:15672` (management UI), documented by `docs/infra/docker-compose.md` (Task 4).

- [ ] **Step 1: Add the `rabbitmq` service to `infra/docker-compose.yml`**

Resulting full file:

```yaml
services:
  rabbitmq:
    image: rabbitmq:management
    container_name: podsentinel-rabbitmq
    env_file: .env
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - rabbitmq-data:/var/lib/rabbitmq

  timescaledb:
    image: timescale/timescaledb:latest-pg16
    container_name: podsentinel-timescaledb
    env_file: .env
    ports:
      - "5432:5432"
    volumes:
      - timescaledb-data:/var/lib/postgresql/data

volumes:
  rabbitmq-data:
  timescaledb-data:
```

- [ ] **Step 2: Verify config parses**

Run:
```bash
cp infra/.env.example infra/.env
docker compose -f infra/docker-compose.yml config --quiet
```
Expected: no output, exit code 0.

- [ ] **Step 3: Bring both services up and verify RabbitMQ**

Run:
```bash
docker compose -f infra/docker-compose.yml up -d
docker compose -f infra/docker-compose.yml ps
curl -sf -u guest:guest http://localhost:15672/api/overview > /dev/null && echo "management API OK"
```
Expected: both services show `running`/healthy in `ps`, and `management API OK` prints (confirms the management UI/API is up with the configured credentials).

- [ ] **Step 4: Tear down**

Run:
```bash
docker compose -f infra/docker-compose.yml down -v
rm infra/.env
```

- [ ] **Step 5: Commit**

```bash
git add infra/docker-compose.yml
git commit -m "build(infra): add RabbitMQ service to docker-compose"
```

---

### Task 4: Documentation

**Files:**
- Create: `docs/infra/docker-compose.md`

**Interfaces:**
- Consumes: the `rabbitmq` and `timescaledb` services and their ports/volumes from Tasks 2–3; the `infra/.env.example` vars from Task 1.
- Produces: nothing consumed by later tasks (this is the last task).

- [ ] **Step 1: Create `docs/infra/docker-compose.md`**

```markdown
# Local RabbitMQ + TimescaleDB (docker-compose)

Guide for the local RabbitMQ and TimescaleDB instances used for development, per the integration testing strategy described in [`architecture.md`](../architecture.md#testing--verification-strategy).

Defined in [`infra/docker-compose.yml`](../../infra/docker-compose.yml): a `rabbitmq` service (with the management UI plugin) and a `timescaledb` service (Postgres + the TimescaleDB extension).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (with the `docker compose` CLI plugin)

## Configure credentials

```bash
cp infra/.env.example infra/.env
```

Adjust `infra/.env` if you want non-default credentials. `infra/.env` is gitignored — never commit it.

## Bring the stack up

```bash
docker compose -f infra/docker-compose.yml up -d
```

## Verify

RabbitMQ management UI: http://localhost:15672 (default `guest`/`guest`, or whatever you set in `infra/.env`).

```bash
docker compose -f infra/docker-compose.yml ps
docker compose -f infra/docker-compose.yml exec timescaledb pg_isready -U postgres
```

Both services should show as running, and `pg_isready` should report `accepting connections`.

## Tear down

```bash
# Stop containers, keep data
docker compose -f infra/docker-compose.yml down

# Stop containers and delete volumes (full reset)
docker compose -f infra/docker-compose.yml down -v
```

## Connection details for app config

| Service | Host (from host machine) | Port | Credentials |
|---|---|---|---|
| RabbitMQ (AMQP) | `localhost` | `5672` | `infra/.env`: `RABBITMQ_DEFAULT_USER` / `RABBITMQ_DEFAULT_PASS` |
| RabbitMQ (management UI) | `localhost` | `15672` | same as above |
| TimescaleDB | `localhost` | `5432` | `infra/.env`: `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` |

If the Go agent later runs inside the local KIND cluster instead of on the host, use `host.docker.internal` in place of `localhost` (works out of the box on macOS with Docker Desktop, which is what KIND runs on).

## Next steps

Schema (`pod_metrics`, `anomalies`) and RabbitMQ exchange/queue topology (`metrics.raw`, `anomalies.detected`) aren't defined yet — they'll be added once the Go agent and Python detector have code that needs them.
```

- [ ] **Step 2: Verify links resolve**

Run:
```bash
test -f docs/architecture.md && echo "architecture.md link OK"
test -f infra/docker-compose.yml && echo "docker-compose.yml link OK"
```
Expected: both print OK (confirms the relative links in the doc point to real files).

- [ ] **Step 3: Commit**

```bash
git add docs/infra/docker-compose.md
git commit -m "docs(infra): add guide for local RabbitMQ/TimescaleDB compose stack"
```
