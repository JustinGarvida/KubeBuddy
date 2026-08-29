# Go Agent

Guide for configuring and running the Go agent (`go/`) locally. See
[`architecture.md`](architecture.md#go-agent) for its role in the
overall system.

**Current stage**: core ingestion path implemented — the agent polls
Kubernetes (pod status/restarts + CPU/memory usage) on an interval and
writes every sample to Postgres/TimescaleDB; REST API endpoints read
real data. RabbitMQ pub/sub and Python anomaly detection are follow-up
work (see "Next steps" below).

## Prerequisites

- [Go](https://go.dev/doc/install) 1.26+ (the module's `go.mod` pins
  `go 1.26.0` — raised from `1.23.0` by `k8s.io/client-go`'s minimum
  Go version; with `GOTOOLCHAIN=auto`, the default, `go build`/`go
  run` download it automatically if your local toolchain is older)
- [Docker](https://docs.docker.com/get-docker/), if building/running the container image

## Configuration

The agent reads configuration from environment variables — copy
[`go/.env.example`](../go/.env.example) to `go/.env` as a reference
for local development (the agent does not load `.env` files itself;
export the variables or use a tool like `direnv`).

| Variable    | Default | Description                                  |
|-------------|---------|-----------------------------------------------|
| `PORT`      | `8080`  | TCP port the REST API listens on               |
| `LOG_LEVEL` | `info`  | Minimum log level: `debug`, `info`, `warn`, `error` |
| `POSTGRES_DSN` | *(none — required)* | Postgres/TimescaleDB connection string |
| `WATCH_NAMESPACES` | *(empty = all namespaces)* | Comma-separated namespace allow-list |
| `POLL_INTERVAL` | `15s` | How often to poll the Kubernetes APIs |

## Run locally

```bash
cd go
go run ./cmd/agent
```

Or build a binary:

```bash
cd go
go build -o bin/agent ./cmd/agent
PORT=8080 LOG_LEVEL=debug ./bin/agent
```

## Run via Docker

```bash
cd go
docker build -t podsentinel-go-agent .
docker run --rm -p 8080:8080 -e LOG_LEVEL=debug podsentinel-go-agent
```

## Verify

```bash
curl localhost:8080/health
curl localhost:8080/api/v1/pods
curl localhost:8080/api/v1/pods/default/example-pod
curl localhost:8080/api/v1/pods/default/example-pod/anomalies
```

## Endpoint reference

| Method | Path                                          | Description                              |
|--------|------------------------------------------------|-------------------------------------------|
| GET    | `/health`                                       | Liveness check                            |
| GET    | `/api/v1/pods`                                  | List pods (dashboard pod list view)       |
| GET    | `/api/v1/pods/{namespace}/{pod}`                | Pod detail with recent metrics            |
| GET    | `/api/v1/pods/{namespace}/{pod}/anomalies`      | Anomaly history for a pod                 |

Response shapes are defined in `go/internal/models`; `/api/v1/pods/{namespace}/{pod}/anomalies`
returns an empty array until the Python anomaly detector (a later
build-order step) exists to write to it.

## Packages

- `cmd/agent` — entrypoint: wires config, logging, and the HTTP server; handles graceful shutdown on `SIGINT`/`SIGTERM`.
- `internal/config` — environment-variable configuration loading.
- `internal/logging` — `log/slog` JSON logger setup.
- `internal/api` — chi router, middleware, and HTTP handlers.
- `internal/models` — API response types, mirroring the eventual `pod_metrics`/`anomalies` Postgres schema.
- `internal/k8s` — builds Kubernetes clients (in-cluster or kubeconfig fallback), polls pods + metrics, and joins them per cycle.
- `internal/store` — Postgres/TimescaleDB schema migrations and CRUD queries.
- `internal/ingest` — orchestrates one poll-and-persist cycle, with per-pod fault isolation.

## Next steps

Tracked as follow-up work, not yet implemented:

- RabbitMQ publishing (`metrics.raw`) and consuming (`anomalies.detected`).
- Python anomaly detection, which will populate the (currently empty) `anomalies` table.
- Deploying the agent itself into the local KIND cluster (RBAC ServiceAccount/ClusterRole for pod + `metrics.k8s.io` access, a Deployment manifest) — today it runs on the host against KIND via kubeconfig.
