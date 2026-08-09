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
