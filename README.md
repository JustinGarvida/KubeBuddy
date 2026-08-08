# KubeBuddy

KubeBuddy monitors Kubernetes pod CPU and memory usage, detects anomalies, and surfaces them in a live dashboard.

## Tech Stack

- **Go** — metrics ingestion agent, RabbitMQ publisher/consumer, REST API
- **Python** — statistical anomaly detection
- **RabbitMQ** — event bus between ingestion and detection, and for anomaly events
- **PostgreSQL / TimescaleDB** — storage for raw metrics and anomaly history
- **React + TypeScript** — dashboard frontend

## Architecture

Go polls the Kubernetes Metrics API and writes raw pod metrics to Postgres, while publishing per-pod metric messages to RabbitMQ. A Python service consumes those messages, runs statistical anomaly detection, and publishes anomaly events back to RabbitMQ. Go consumes those events, persists them, and serves everything to the React dashboard through a REST API.

See [`docs/architecture.md`](docs/architecture.md) for the full design, including component responsibilities, error handling, testing strategy, and the suggested build order.

## Status

Early stage — this repo currently contains design docs only, no application code yet. Setup and run instructions will be added here as each component (Go agent, Python detector, dashboard) is implemented.

## Planned Repo Layout

```
go/         # Go agent: metrics ingestion, RabbitMQ pub/sub, REST API
python/     # Python anomaly detection service
frontend/   # React + TypeScript dashboard
docs/       # Design and architecture documentation
```
