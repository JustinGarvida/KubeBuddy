# Design: Go agent core ingestion path

**Issue**: none yet
**Branch**: to be created for implementation (e.g. `feat/go-agent-ingestion-path`)
**Date**: 2026-08-28

## Problem

The Go agent (`go/`) is currently a scaffold: `cmd/agent` wires up config, logging, and a chi router, but every REST endpoint (`internal/api/handlers.go`) returns hardcoded placeholder data. This is build-order step 1 in `docs/architecture.md`: prove the core ingestion path end-to-end — Kubernetes polling, direct Postgres writes, and a REST API backed by real data — before RabbitMQ, Python anomaly detection, or the dashboard's pod list view are touched. Those are separate, later sub-projects.

## Scope

### A. Polling (`go/internal/k8s`)

- A `Poller` ticks on a configurable interval (`POLL_INTERVAL` env var, default `15s`).
- Each tick queries **two** Kubernetes APIs and joins the results by `namespace/name`:
  - Core `v1.Pod` list (`client-go` `kubernetes.Clientset`) — for identity, `status.phase`, and restart count (sum of `ContainerStatuses[].RestartCount` across a pod's containers).
  - `metrics.k8s.io` `PodMetrics` list (`k8s.io/metrics` `versioned.Clientset`) — for CPU/memory usage.
  - The join is necessary because the Metrics API only reports resource usage, not pod identity/status/restarts — both are needed for `PodSummary`/`PodDetail`, and the restart-count/status signal is also expected to be useful as a future anomaly-detection feature.
- Namespace scope: `WATCH_NAMESPACES` env var, comma-separated allow-list. Empty/unset means "watch all namespaces" — a namespace config table in Postgres was considered and rejected for now (see "Rejected approaches" below).
- Kubernetes client auth: try `rest.InClusterConfig()` first, fall back to `clientcmd` loading `KUBECONFIG`/`~/.kube/config`. One code path works both in-cluster (production, per architecture.md's RBAC design) and on a developer's host machine against the local KIND cluster.
- Fault isolation (matches architecture.md): a pod missing from one list, or any per-pod list/scrape error, is logged and skipped — it does not fail the rest of that polling cycle.

### B. Postgres schema & writer (`go/internal/store`)

- Migrations in `go/migrations`, run automatically at agent startup via `golang-migrate` (idempotent).
  - `pod_metrics` — TimescaleDB hypertable, partitioned on `time`: `time timestamptz`, `namespace text`, `pod text`, `cpu double precision`, `memory double precision`, `status text`, `restart_count int`. Status/restart count are denormalized onto the same row as CPU/memory (not a separate pod-identity table) — they're written once per pod per poll cycle alongside the metrics anyway, so this avoids an extra join for no benefit. Index on `(namespace, pod, time desc)` for the pod-detail query.
  - `anomalies` — `time timestamptz`, `namespace text`, `pod text`, `metric text`, `value double precision`, `baseline double precision`, `severity text`. Created now even though nothing writes to it until the Python detector exists (later phase) — this lets the anomalies endpoint return real (empty) query results instead of a hardcoded placeholder.
- `pgxpool.Pool` built once at startup from `POSTGRES_DSN` (required env var, no default — matches `infra/.env`'s TimescaleDB credentials).
- `store.Store` exposes `InsertPodMetric`, `ListPods`, `GetPodMetrics(namespace, pod, since)`, `ListAnomalies(namespace, pod)`.
- The poller calls `InsertPodMetric` directly after each successful join, independent of RabbitMQ (not built yet) — matches architecture.md's "Postgres writes don't depend on the queue" decision.
- A failed insert for one pod is logged and skipped, not fatal to the rest of the cycle's writes (same fault-isolation principle as polling).

### C. REST API + config

- `handlers` gains a `store.Store` dependency (in addition to the existing `logger`), injected in `main.go`.
- `GET /api/v1/pods` → `store.ListPods()`: latest known row per pod (e.g. `DISTINCT ON (namespace, pod) ... ORDER BY time DESC`).
- `GET /api/v1/pods/{namespace}/{pod}` → `store.GetPodMetrics(namespace, pod, since)`: time-series for a fixed 1h lookback window; no `since` query param or pagination yet (YAGNI at this stage).
- `GET /api/v1/pods/{namespace}/{pod}/anomalies` → `store.ListAnomalies(namespace, pod)`: returns `[]` until the Python detector is built — expected and correct for this phase.
- New config (`go/internal/config`): `POSTGRES_DSN` (required), `WATCH_NAMESPACES` (default `""` = all namespaces), `POLL_INTERVAL` (default `15s`).
- Module path: `go.mod`'s `module github.com/JustinGarvida/PodSentinel/go` becomes `module podsentinel`, and every internal import (`cmd/agent`, `internal/api`, etc.) updates accordingly — purely cosmetic (this module is never `go get`-imported externally), done now while several new packages (`internal/k8s`, `internal/store`) are being added anyway.

### D. metrics-server (KIND) — already implemented and verified

Done ahead of the rest of this spec, since it was a standalone local-environment blocker:

- [`infra/kind/metrics-server.yaml`](../../../infra/kind/metrics-server.yaml) — upstream metrics-server manifest pinned to `v0.9.0` (matching this repo's convention of pinning infra image tags), with `--kubelet-insecure-tls` added to the container args (required for KIND — its kubelet serving certs aren't signed for the hostnames/IPs metrics-server validates by default, so the Deployment never reaches `Ready` without it).
- [`docs/infra/kind.md`](../../infra/kind.md) — install/verify steps added.
- Verified against the running `kind-podsentinel` cluster: `kubectl top nodes` / `kubectl top pods -A` return real data.

### Testing

- **Poller unit tests**: `k8s.io/client-go/kubernetes/fake` + the metrics API's fake clientset, feeding synthetic `Pod`/`PodMetrics` objects. Assert the join logic and per-pod fault isolation (one bad/missing pod doesn't drop the rest of the cycle).
- **Store/handler tests**: run against the real `infra/docker-compose.yml` TimescaleDB instance (already used per architecture.md's testing strategy) rather than mocking Postgres — migrations apply automatically, so a docker-composed Postgres is the test boundary. Handlers tested via `httptest` against the router with a test-DB-backed `store.Store`, seeded with known rows.
- **Integration test**: runs against the developer's already-running real infrastructure — the `kind-podsentinel` KIND cluster (real pod/metrics data) and `infra/docker-compose.yml`'s TimescaleDB (real writes) — rather than fakes. This is a deliberate tradeoff (see "Rejected approaches"): it's not CI-portable as written, but matches the current local dev setup and gives higher-fidelity verification than fakes would. The test is gated behind a build tag (e.g. `-tags=integration`) so `go test ./...` doesn't fail in environments without a `kind-podsentinel` context.

## Rejected approaches

- **Namespace scope stored in Postgres**, editable at runtime, instead of a static `WATCH_NAMESPACES` env var. Rejected because it only pays off once something can edit it at runtime (an admin endpoint/UI), which doesn't exist and isn't in scope here — it would add a migration, a cache-or-requery-per-cycle concern, and nothing yet to edit the rows with. Noted as a future-work candidate once a dashboard settings surface exists.
- **YAML config file** for `WATCH_NAMESPACES` (optionally falling back to "watch all" if absent). Rejected in favor of keeping all current config as env vars (`PORT`, `LOG_LEVEL`, and now `POSTGRES_DSN`/`WATCH_NAMESPACES`/`POLL_INTERVAL`) — introducing a second config source for just one setting added complexity without a clear win at this scale.
- **Integration test on fake clientsets** (originally proposed for CI portability/determinism). Rejected in favor of running against the real KIND cluster and real docker-compose Postgres, since both are already part of the developer's local workflow; CI portability can be revisited later if/when this project gets automated CI.

## Out of scope

- RabbitMQ publishing (`metrics.raw`) / consuming (`anomalies.detected`) — build-order step 2.
- Python anomaly detection — build-order step 3.
- Dashboard pod list view (React/TS) — separate sub-project, blocked on this spec's REST API shape existing.
- Deploying the Go agent into the KIND cluster (manifests, RBAC ServiceAccount/ClusterRole for `metrics.k8s.io`/pod access) — the agent runs on the host against KIND via kubeconfig for now; in-cluster deployment is later follow-up work per `docs/kube-agent.md`.
- Namespace config editable at runtime (see "Rejected approaches").

## Testing / Verification

- `go test ./...` passes, including poller unit tests (fake clientsets) and store/handler tests (docker-compose TimescaleDB).
- `go test -tags=integration ./...` passes against the running `kind-podsentinel` cluster + `infra/docker-compose.yml`.
- Manual: `go run ./cmd/agent` against the local KIND cluster + docker-compose Postgres; `curl localhost:8080/api/v1/pods` returns real pod data (not the old placeholder); create a workload in a watched namespace and confirm it appears within one poll interval; confirm a namespace excluded via `WATCH_NAMESPACES` never appears.
