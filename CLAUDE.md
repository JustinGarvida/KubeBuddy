# CLAUDE.md

Guidance for Claude Code sessions working in this repository.

## Project Summary

KubeBuddy monitors Kubernetes pod CPU/memory usage, detects anomalies with a statistical model, and displays them in a React/TypeScript dashboard. It's an experimental, single-cluster project intentionally combining Go, Python, RabbitMQ, and React/TS.

**Current stage**: design/docs only. No application code exists yet — see the suggested build order in `docs/architecture.md` before starting implementation work.

## Tech Stack & Why

- **Go** — chosen for the metrics ingestion agent because it pairs naturally with `client-go` for talking to the Kubernetes API, and is fast enough to run as a lightweight in-cluster Deployment. The same Go binary handles polling, Postgres writes, RabbitMQ pub/sub, and the REST API — deliberately one service, not several, to keep this experimental project's ops surface small.
- **Python** — chosen for anomaly detection to make use of its statistics/ML ecosystem, and because the user specifically wanted to experiment with Python for the ML side of this project.
- **RabbitMQ** — the event bus between Go (ingestion) and Python (detection), and for publishing anomaly events. Chosen specifically as an experiment with message-queue-driven architecture, not because scale requires it.
- **PostgreSQL / TimescaleDB** — source of truth for raw metrics and anomaly history; TimescaleDB extension for efficient time-series storage/queries.
- **React + TypeScript** — dashboard frontend, reading from the Go REST API.

## Planned Repo Structure

```
go/         # Go agent: metrics ingestion, RabbitMQ pub/sub, REST API
python/     # Python anomaly detection service
frontend/   # React + TypeScript dashboard
docs/       # Design and architecture documentation
```

## Key Architectural Decisions (preserve this context)

- **Per-pod RabbitMQ messages, not batched.** Deliberate choice: one malformed payload should only affect one pod, not a whole batch. Malformed messages go to a dead-letter queue rather than crashing a consumer.
- **Go is the sole Postgres writer.** Python never writes to Postgres directly — it only publishes anomaly events to RabbitMQ, which Go consumes and persists. This keeps all database access centralized in one service.
- **Statistical baseline before ML.** Anomaly detection starts with z-score/EWMA-based deviation from a pod's rolling recent baseline — explainable, no training data needed. An ML model (e.g. Isolation Forest) is a documented future upgrade, not the starting point.
- **Postgres writes don't depend on RabbitMQ.** Go writes raw metrics to Postgres directly during polling, independent of whether the RabbitMQ publish succeeds — core recording survives queue outages.
- **RabbitMQ's `anomalies.detected` exchange has no built-in alert consumer yet.** Only Go consumes it today (to persist anomalies for the dashboard). The schema is designed so a future alerting consumer (Slack/email/webhook) could bind its own queue without changing the publisher.

## Reference

Full architecture, component responsibilities, error handling, testing strategy, and the suggested build order live in [`docs/architecture.md`](docs/architecture.md) — treat it as the source of truth when implementing any component.
