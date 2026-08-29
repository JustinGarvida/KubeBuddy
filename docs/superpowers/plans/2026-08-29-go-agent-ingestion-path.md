# Go Agent Core Ingestion Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Go agent's placeholder REST responses with a real ingestion path: poll Kubernetes (pod identity/status + metrics) on an interval, write every sample to Postgres/TimescaleDB, and serve the REST API from that data.

**Architecture:** A `Poller` (package `k8s`) lists `v1.Pod` and `PodMetrics` per watched namespace on each tick and joins them into `PodSample`s. An `ingest.Run` orchestration function persists each sample via `store.Store.InsertPodMetric` (package `store`, backed by `pgxpool` + TimescaleDB, schema applied via embedded `golang-migrate` migrations). `cmd/agent` wires a background poll loop alongside the existing HTTP server, whose handlers (package `api`) now read from `store.Store` instead of returning hardcoded values.

**Tech Stack:** Go 1.23 (toolchain auto-downloaded per `go.mod`'s `go 1.23.0` pin — expect a one-time download on first build), `client-go` + `k8s.io/metrics` (Kubernetes), `pgx/v5` + `pgxpool` (Postgres), `golang-migrate/v4` (schema migrations), `chi` (already in use), stdlib `testing`.

**Spec:** [`docs/superpowers/specs/2026-08-28-go-agent-ingestion-path-design.md`](../specs/2026-08-28-go-agent-ingestion-path-design.md)

## Global Constraints

- Module path becomes `podsentinel` (was `github.com/JustinGarvida/PodSentinel/go`) — every internal import updates accordingly.
- `POSTGRES_DSN` env var is required, no default; left empty it surfaces as a connection error at agent startup, not validated separately.
- `WATCH_NAMESPACES` env var: comma-separated namespace allow-list, trimmed; empty/unset means watch all namespaces.
- `POLL_INTERVAL` env var: default `15s`; an invalid value falls back to the default rather than erroring.
- Fault isolation everywhere in the ingestion path (namespace listing, pod/metrics join, Postgres insert): log and skip the one bad item, never fail the whole cycle.
- `pod_metrics` denormalizes `status`/`restart_count` onto the same row as `cpu`/`memory` — no separate pod-identity table.
- `anomalies` table is created now but stays unwritten until the Python detector exists (later build-order step) — `ListAnomalies` returning `[]` is correct, not a bug.
- `GetPodMetrics` uses a fixed 1-hour lookback window; no `since` param or pagination yet.
- Schema migrations live at `go/internal/store/migrations/*.sql`, embedded via `go:embed`, and run automatically (idempotently) every time `store.Open` is called.
- Local dev infra is assumed already running for store/handler unit tests and the integration test: `infra/docker-compose.yml`'s `timescaledb` service (`postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable`, overridable via `TEST_POSTGRES_DSN`), and the `kind-podsentinel` KIND cluster with `metrics-server` installed (already done — see `infra/kind/metrics-server.yaml`).
- Endpoint paths are unchanged from the existing scaffold: `GET /health`, `GET /api/v1/pods`, `GET /api/v1/pods/{namespace}/{pod}`, `GET /api/v1/pods/{namespace}/{pod}/anomalies`.

---

## Task 1: Rename the Go module path

**Files:**
- Modify: `go/go.mod`
- Modify: `go/cmd/agent/main.go`
- Modify: `go/internal/api/handlers.go`

**Interfaces:**
- Produces: module path `podsentinel` — every task after this one imports internal packages as `podsentinel/internal/...`.

This is a pure mechanical rename with no behavior change, so there's no new test to write — verification is that the module still builds under the new path.

- [ ] **Step 1: Edit `go/go.mod`**

Change:
```
module github.com/JustinGarvida/PodSentinel/go
```
to:
```
module podsentinel
```

- [ ] **Step 2: Update imports in `go/cmd/agent/main.go`**

Change:
```go
	"github.com/JustinGarvida/PodSentinel/go/internal/api"
	"github.com/JustinGarvida/PodSentinel/go/internal/config"
	"github.com/JustinGarvida/PodSentinel/go/internal/logging"
```
to:
```go
	"podsentinel/internal/api"
	"podsentinel/internal/config"
	"podsentinel/internal/logging"
```

- [ ] **Step 3: Update the import in `go/internal/api/handlers.go`**

Change:
```go
	"github.com/JustinGarvida/PodSentinel/go/internal/models"
```
to:
```go
	"podsentinel/internal/models"
```

- [ ] **Step 4: Verify the build**

Run (from `go/`): `go build ./...`
Expected: succeeds with no output. (First run may take longer than usual — it downloads the Go 1.23 toolchain per `go.mod`'s pin.)

- [ ] **Step 5: Commit**

```bash
cd go
git add go.mod cmd/agent/main.go internal/api/handlers.go
git commit -m "refactor(go-agent): rename module path to podsentinel"
```

---

## Task 2: Extend agent configuration

**Files:**
- Modify: `go/internal/config/config.go`
- Test: `go/internal/config/config_test.go` (new)

**Interfaces:**
- Produces: `config.Config` gains `PostgresDSN string`, `WatchNamespaces []string`, `PollInterval time.Duration`. `config.Load()` signature unchanged (`func Load() Config`).

- [ ] **Step 1: Write the failing tests**

Create `go/internal/config/config_test.go`:

```go
package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("WATCH_NAMESPACES", "")
	t.Setenv("POLL_INTERVAL", "")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.PostgresDSN != "" {
		t.Errorf("PostgresDSN = %q, want empty", cfg.PostgresDSN)
	}
	if cfg.WatchNamespaces != nil {
		t.Errorf("WatchNamespaces = %v, want nil", cfg.WatchNamespaces)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 15*time.Second)
	}
}

func TestLoad_WatchNamespacesParsesAndTrims(t *testing.T) {
	t.Setenv("WATCH_NAMESPACES", " default , podsentinel-demo ,,staging")

	cfg := Load()

	want := []string{"default", "podsentinel-demo", "staging"}
	if len(cfg.WatchNamespaces) != len(want) {
		t.Fatalf("WatchNamespaces = %v, want %v", cfg.WatchNamespaces, want)
	}
	for i, ns := range want {
		if cfg.WatchNamespaces[i] != ns {
			t.Errorf("WatchNamespaces[%d] = %q, want %q", i, cfg.WatchNamespaces[i], ns)
		}
	}
}

func TestLoad_PollIntervalCustom(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "30s")

	cfg := Load()

	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 30*time.Second)
	}
}

func TestLoad_PollIntervalInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "not-a-duration")

	cfg := Load()

	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval = %v, want default %v", cfg.PollInterval, 15*time.Second)
	}
}

func TestLoad_PostgresDSNPassthrough(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/podsentinel")

	cfg := Load()

	want := "postgres://user:pass@localhost:5432/podsentinel"
	if cfg.PostgresDSN != want {
		t.Errorf("PostgresDSN = %q, want %q", cfg.PostgresDSN, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/config/...`
Expected: FAIL — compile error, `Config` has no field `PostgresDSN` (etc.).

- [ ] **Step 3: Implement the config changes**

Replace `go/internal/config/config.go` with:

```go
// Package config loads the Go agent's runtime configuration from
// environment variables.
package config

import (
	"os"
	"strings"
	"time"
)

// Config holds the agent's runtime settings.
type Config struct {
	// Port is the TCP port the REST API listens on.
	Port string
	// LogLevel is the minimum level logged (debug, info, warn, error).
	LogLevel string
	// PostgresDSN is the connection string for the Postgres/TimescaleDB
	// instance the agent writes metrics and anomalies to. Required —
	// left empty if unset, which surfaces as a connection error at
	// startup rather than being validated here.
	PostgresDSN string
	// WatchNamespaces restricts polling to these namespaces. Empty
	// means watch all namespaces.
	WatchNamespaces []string
	// PollInterval is how often the agent polls the Kubernetes APIs.
	PollInterval time.Duration
}

// Load reads configuration from environment variables, falling back to
// defaults for anything unset.
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		PostgresDSN:     getEnv("POSTGRES_DSN", ""),
		WatchNamespaces: parseNamespaces(getEnv("WATCH_NAMESPACES", "")),
		PollInterval:    parseDuration(getEnv("POLL_INTERVAL", "15s"), 15*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseNamespaces splits a comma-separated namespace list, trimming
// whitespace and dropping empty entries. An empty input yields a nil
// slice, meaning "watch all namespaces".
func parseNamespaces(raw string) []string {
	if raw == "" {
		return nil
	}
	var namespaces []string
	for _, ns := range strings.Split(raw, ",") {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces
}

// parseDuration parses a duration string, falling back to the given
// default if it's empty or invalid.
func parseDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/config/...`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
cd go
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(go-agent): add postgres, namespace, and poll-interval config"
```

---

## Task 3: Kubernetes client construction

**Files:**
- Create: `go/internal/k8s/client.go`
- Test: `go/internal/k8s/client_test.go`

**Interfaces:**
- Produces: `k8s.Clients{Core kubernetes.Interface; Metrics metricsv.Interface}`, `k8s.BuildClients() (*Clients, error)`.

- [ ] **Step 1: Add the Kubernetes client dependencies**

Run (from `go/`):
```bash
go get k8s.io/client-go@latest k8s.io/metrics@latest
```
Expected: `go.mod`/`go.sum` updated with `k8s.io/client-go` and `k8s.io/metrics` (and their transitive `k8s.io/api`, `k8s.io/apimachinery`).

- [ ] **Step 2: Write the failing tests**

Create `go/internal/k8s/client_test.go`:

```go
package k8s

import (
	"errors"
	"testing"

	"k8s.io/client-go/rest"
)

func TestBuildRestConfig_PrefersInCluster(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return &rest.Config{Host: "https://in-cluster"}, nil
	}
	outOfCluster := func() (*rest.Config, error) {
		t.Fatal("outOfCluster loader should not be called when in-cluster succeeds")
		return nil, nil
	}

	cfg, err := buildRestConfig(inCluster, outOfCluster)
	if err != nil {
		t.Fatalf("buildRestConfig() error = %v", err)
	}
	if cfg.Host != "https://in-cluster" {
		t.Errorf("Host = %q, want %q", cfg.Host, "https://in-cluster")
	}
}

func TestBuildRestConfig_FallsBackToKubeconfig(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return nil, errors.New("not running in a pod")
	}
	outOfCluster := func() (*rest.Config, error) {
		return &rest.Config{Host: "https://kind-podsentinel"}, nil
	}

	cfg, err := buildRestConfig(inCluster, outOfCluster)
	if err != nil {
		t.Fatalf("buildRestConfig() error = %v", err)
	}
	if cfg.Host != "https://kind-podsentinel" {
		t.Errorf("Host = %q, want %q", cfg.Host, "https://kind-podsentinel")
	}
}

func TestBuildRestConfig_ErrorsWhenBothFail(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return nil, errors.New("not running in a pod")
	}
	outOfCluster := func() (*rest.Config, error) {
		return nil, errors.New("no kubeconfig found")
	}

	_, err := buildRestConfig(inCluster, outOfCluster)
	if err == nil {
		t.Fatal("buildRestConfig() error = nil, want error when both loaders fail")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/k8s/...`
Expected: FAIL — compile error, `buildRestConfig` is undefined.

- [ ] **Step 4: Implement the client construction**

Create `go/internal/k8s/client.go`:

```go
// Package k8s builds Kubernetes API clients and polls pod/metrics data.
package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Clients bundles the two clientsets the poller needs.
type Clients struct {
	Core    kubernetes.Interface
	Metrics metricsv.Interface
}

// BuildClients constructs Clients using in-cluster credentials if
// available, falling back to the local kubeconfig (KUBECONFIG, or
// ~/.kube/config) for development against a cluster like KIND.
func BuildClients() (*Clients, error) {
	restConfig, err := buildRestConfig(rest.InClusterConfig, loadKubeconfig)
	if err != nil {
		return nil, err
	}

	core, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building core clientset: %w", err)
	}

	metrics, err := metricsv.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building metrics clientset: %w", err)
	}

	return &Clients{Core: core, Metrics: metrics}, nil
}

// buildRestConfig tries inCluster first, falling back to
// outOfCluster. Both loaders are injected so the fallback ordering can
// be unit tested without real cluster credentials or a kubeconfig file.
func buildRestConfig(inCluster, outOfCluster func() (*rest.Config, error)) (*rest.Config, error) {
	cfg, err := inCluster()
	if err == nil {
		return cfg, nil
	}

	cfg, kubeErr := outOfCluster()
	if kubeErr != nil {
		return nil, fmt.Errorf("no in-cluster config (%v) and failed to load kubeconfig (%w)", err, kubeErr)
	}

	return cfg, nil
}

// loadKubeconfig loads a *rest.Config from KUBECONFIG, or ~/.kube/config
// if KUBECONFIG is unset.
func loadKubeconfig() (*rest.Config, error) {
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		path = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", path)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/k8s/...`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
cd go
git add go.mod go.sum internal/k8s/client.go internal/k8s/client_test.go
git commit -m "feat(go-agent): build kubernetes clients with in-cluster/kubeconfig fallback"
```

---

## Task 4: Pod + metrics join logic

**Files:**
- Create: `go/internal/k8s/join.go`
- Test: `go/internal/k8s/join_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `k8s.PodSample{Namespace, Name, Status string; RestartCount int32; CPU, Memory float64; Timestamp time.Time}`, `joinPodsAndMetrics(pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics, now time.Time, logger *slog.Logger) []PodSample` (unexported — used by the `Poller` in Task 5, same package).

- [ ] **Step 1: Write the failing tests**

Create `go/internal/k8s/join_test.go`:

```go
package k8s

import (
	"io"
	"log/slog"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestJoinPodsAndMetrics_JoinsByNamespaceAndName(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 2},
					{RestartCount: 1},
				},
			},
		},
	}

	metrics := []metricsv1beta1.PodMetrics{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Containers: []metricsv1beta1.ContainerMetrics{
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				}},
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				}},
			},
		},
	}

	samples := joinPodsAndMetrics(pods, metrics, now, testLogger())

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(samples))
	}
	got := samples[0]
	if got.Namespace != "default" || got.Name != "web-1" {
		t.Errorf("identity = %s/%s, want default/web-1", got.Namespace, got.Name)
	}
	if got.Status != "Running" {
		t.Errorf("Status = %q, want %q", got.Status, "Running")
	}
	if got.RestartCount != 3 {
		t.Errorf("RestartCount = %d, want 3", got.RestartCount)
	}
	if got.CPU != 0.15 {
		t.Errorf("CPU = %v, want 0.15", got.CPU)
	}
	wantMemory := float64(96 * 1024 * 1024)
	if got.Memory != wantMemory {
		t.Errorf("Memory = %v, want %v", got.Memory, wantMemory)
	}
	if !got.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, now)
	}
}

func TestJoinPodsAndMetrics_SkipsPodWithNoMetrics(t *testing.T) {
	now := time.Now()
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "no-metrics-yet"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
	}
	metrics := []metricsv1beta1.PodMetrics{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
	}

	samples := joinPodsAndMetrics(pods, metrics, now, testLogger())

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1 (the pod missing metrics should be skipped, not fail the batch)", len(samples))
	}
	if samples[0].Name != "web-1" {
		t.Errorf("samples[0].Name = %q, want %q", samples[0].Name, "web-1")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/k8s/...`
Expected: FAIL — compile error, `PodSample`/`joinPodsAndMetrics` undefined.

- [ ] **Step 3: Implement the join logic**

Create `go/internal/k8s/join.go`:

```go
package k8s

import (
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// PodSample is one pod's joined identity, status, and resource usage
// for a single poll cycle.
type PodSample struct {
	Namespace    string
	Name         string
	Status       string
	RestartCount int32
	CPU          float64
	Memory       float64
	Timestamp    time.Time
}

// joinPodsAndMetrics joins pod identity/status (from the core API) with
// resource usage (from the metrics API) by namespace/name. A pod with
// no matching metrics entry — e.g. too new to have been scraped yet —
// is logged and skipped rather than failing the whole cycle.
func joinPodsAndMetrics(pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics, now time.Time, logger *slog.Logger) []PodSample {
	metricsByKey := make(map[string]metricsv1beta1.PodMetrics, len(metrics))
	for _, m := range metrics {
		metricsByKey[m.Namespace+"/"+m.Name] = m
	}

	samples := make([]PodSample, 0, len(pods))
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		m, ok := metricsByKey[key]
		if !ok {
			logger.Warn("no metrics for pod, skipping", "namespace", pod.Namespace, "pod", pod.Name)
			continue
		}

		var cpu, memory float64
		for _, c := range m.Containers {
			cpu += c.Usage.Cpu().AsApproximateFloat64()
			memory += c.Usage.Memory().AsApproximateFloat64()
		}

		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}

		samples = append(samples, PodSample{
			Namespace:    pod.Namespace,
			Name:         pod.Name,
			Status:       string(pod.Status.Phase),
			RestartCount: restarts,
			CPU:          cpu,
			Memory:       memory,
			Timestamp:    now,
		})
	}

	return samples
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run (from `go/`): `go mod tidy && go test ./internal/k8s/...`
Expected: PASS (5 tests total — the 3 from Task 3 plus these 2).

- [ ] **Step 5: Commit**

```bash
cd go
git add go.mod go.sum internal/k8s/join.go internal/k8s/join_test.go
git commit -m "feat(go-agent): join pod identity/status with metrics usage per cycle"
```

---

## Task 5: Poller

**Files:**
- Create: `go/internal/k8s/poller.go`
- Test: `go/internal/k8s/poller_test.go`

**Interfaces:**
- Consumes: `k8s.Clients` (Task 3), `PodSample`/`joinPodsAndMetrics` (Task 4).
- Produces: `k8s.Poller{Clients *Clients; Namespaces []string; Logger *slog.Logger; Now func() time.Time}`, `k8s.NewPoller(clients *Clients, namespaces []string, logger *slog.Logger) *Poller`, `(*Poller).Poll(ctx context.Context) []PodSample`.

- [ ] **Step 1: Write the failing tests**

Create `go/internal/k8s/poller_test.go`:

```go
package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestPoller_Poll_JoinsAcrossWatchedNamespaces(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)
	metrics := metricsfake.NewSimpleClientset(
		&metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Containers: []metricsv1beta1.ContainerMetrics{
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				}},
			},
		},
		&metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"},
		},
	)

	poller := &Poller{
		Clients:    &Clients{Core: core, Metrics: metrics},
		Namespaces: []string{"default"},
		Logger:     testLogger(),
		Now:        func() time.Time { return time.Unix(0, 0) },
	}

	samples := poller.Poll(context.Background())

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1 (kube-system is not watched)", len(samples))
	}
	if samples[0].Namespace != "default" || samples[0].Name != "web-1" {
		t.Errorf("got %s/%s, want default/web-1", samples[0].Namespace, samples[0].Name)
	}
}

func TestPoller_Poll_EmptyNamespacesMeansAll(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)
	metrics := metricsfake.NewSimpleClientset(
		&metricsv1beta1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&metricsv1beta1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)

	poller := &Poller{
		Clients: &Clients{Core: core, Metrics: metrics},
		Logger:  testLogger(),
		Now:     time.Now,
	}

	samples := poller.Poll(context.Background())

	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2 (no namespace filter means watch all)", len(samples))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/k8s/...`
Expected: FAIL — compile error, `Poller` undefined.

- [ ] **Step 3: Add the fake-clientset test dependencies**

Run (from `go/`): `go mod tidy`
(`k8s.io/client-go/kubernetes/fake` and `k8s.io/metrics/pkg/client/clientset/versioned/fake` are part of the modules already fetched in Task 3.)

- [ ] **Step 4: Implement the poller**

Create `go/internal/k8s/poller.go`:

```go
package k8s

import (
	"context"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Poller lists pods and their metrics from the Kubernetes API on each
// Poll call, joining them into PodSamples.
type Poller struct {
	Clients    *Clients
	Namespaces []string // empty means all namespaces
	Logger     *slog.Logger
	Now        func() time.Time
}

// NewPoller builds a Poller with real clientsets and time.Now.
func NewPoller(clients *Clients, namespaces []string, logger *slog.Logger) *Poller {
	return &Poller{
		Clients:    clients,
		Namespaces: namespaces,
		Logger:     logger,
		Now:        time.Now,
	}
}

// Poll lists pods and pod metrics across the configured namespaces and
// returns the joined samples. A list failure for one namespace is
// logged and skipped rather than failing the whole poll.
func (p *Poller) Poll(ctx context.Context) []PodSample {
	namespaces := p.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	now := p.Now()
	var samples []PodSample

	for _, ns := range namespaces {
		pods, err := p.Clients.Core.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			p.Logger.Error("listing pods failed, skipping namespace", "namespace", ns, "error", err)
			continue
		}

		metrics, err := p.Clients.Metrics.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			p.Logger.Error("listing pod metrics failed, skipping namespace", "namespace", ns, "error", err)
			continue
		}

		samples = append(samples, joinPodsAndMetrics(pods.Items, metrics.Items, now, p.Logger)...)
	}

	return samples
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/k8s/...`
Expected: PASS (7 tests total).

- [ ] **Step 6: Commit**

```bash
cd go
git add go.mod go.sum internal/k8s/poller.go internal/k8s/poller_test.go
git commit -m "feat(go-agent): poll pods and metrics per watched namespace"
```

---

## Task 6: Postgres schema migrations

**Files:**
- Create: `go/internal/store/migrations/0001_init_schema.up.sql`
- Create: `go/internal/store/migrations/0001_init_schema.down.sql`
- Create: `go/internal/store/migrate.go`
- Create: `go/internal/store/testdb_test.go`
- Test: `go/internal/store/migrate_test.go`

**Interfaces:**
- Produces: `store.Migrate(dsn string) error`. Test helpers (package-internal, used by later `store` tests too): `testDSN(t *testing.T) string`, `requireTestDB(t *testing.T) string`.

- [ ] **Step 1: Add the Postgres/migration dependencies**

Run (from `go/`):
```bash
go get github.com/jackc/pgx/v5@latest github.com/golang-migrate/migrate/v4@latest
```

- [ ] **Step 2: Write the migration SQL**

Create `go/internal/store/migrations/0001_init_schema.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS pod_metrics (
    time          TIMESTAMPTZ      NOT NULL,
    namespace     TEXT             NOT NULL,
    pod           TEXT             NOT NULL,
    cpu           DOUBLE PRECISION NOT NULL,
    memory        DOUBLE PRECISION NOT NULL,
    status        TEXT             NOT NULL,
    restart_count INTEGER          NOT NULL
);

SELECT create_hypertable('pod_metrics', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS pod_metrics_namespace_pod_time_idx
    ON pod_metrics (namespace, pod, time DESC);

CREATE TABLE IF NOT EXISTS anomalies (
    time      TIMESTAMPTZ      NOT NULL,
    namespace TEXT             NOT NULL,
    pod       TEXT             NOT NULL,
    metric    TEXT             NOT NULL,
    value     DOUBLE PRECISION NOT NULL,
    baseline  DOUBLE PRECISION NOT NULL,
    severity  TEXT             NOT NULL
);

CREATE INDEX IF NOT EXISTS anomalies_namespace_pod_time_idx
    ON anomalies (namespace, pod, time DESC);
```

Create `go/internal/store/migrations/0001_init_schema.down.sql`:

```sql
DROP TABLE IF EXISTS anomalies;
DROP TABLE IF EXISTS pod_metrics;
```

- [ ] **Step 3: Add the shared test-DB helper**

Create `go/internal/store/testdb_test.go`:

```go
package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDSN returns the Postgres DSN to test against — the docker-compose
// TimescaleDB instance from infra/docker-compose.yml. Set
// TEST_POSTGRES_DSN to override; otherwise this matches
// infra/.env.example's default credentials.
func testDSN(t *testing.T) string {
	t.Helper()
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable"
}

// requireTestDB skips the test if the docker-compose TimescaleDB
// instance (see infra/docker-compose.md) isn't reachable.
func requireTestDB(t *testing.T) string {
	t.Helper()
	dsn := testDSN(t)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping: could not create pool for %s: %v", dsn, err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("skipping: Postgres not reachable at %s (is `docker compose -f infra/docker-compose.yml up -d` running?): %v", dsn, err)
	}

	return dsn
}
```

- [ ] **Step 4: Write the failing migration test**

Create `go/internal/store/migrate_test.go`:

```go
package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	dsn := requireTestDB(t)

	if err := Migrate(dsn); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	for _, table := range []string{"pod_metrics", "anomalies"} {
		var exists bool
		err := pool.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("checking table %q: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created by Migrate()", table)
		}
	}

	// Running Migrate again must be a no-op, not an error — the agent
	// calls this on every startup.
	if err := Migrate(dsn); err != nil {
		t.Fatalf("Migrate() second call error = %v", err)
	}
}
```

- [ ] **Step 5: Run the test to verify it fails**

Run (from `go/`): `go test ./internal/store/...`
Expected: FAIL — compile error, `Migrate` undefined.

- [ ] **Step 6: Implement the migration runner**

Create `go/internal/store/migrate.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending schema migrations. It's safe to call on
// every agent startup — already-applied migrations are no-ops.
func Migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("opening migration connection: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("building postgres migration driver: %w", err)
	}

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "pgx", driver)
	if err != nil {
		return fmt.Errorf("building migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}
```

- [ ] **Step 7: Run the test to verify it passes**

Run (from `go/`): `go mod tidy && go test ./internal/store/...`
Expected: PASS. If it instead reports "skipping: Postgres not reachable", run `docker compose -f infra/docker-compose.yml up -d` from the repo root first.

- [ ] **Step 8: Commit**

```bash
cd go
git add go.mod go.sum internal/store/migrations internal/store/migrate.go internal/store/migrate_test.go internal/store/testdb_test.go
git commit -m "feat(go-agent): add pod_metrics/anomalies schema migrations"
```

---

## Task 7: Store CRUD methods

**Files:**
- Create: `go/internal/store/store.go`
- Test: `go/internal/store/store_test.go`

**Interfaces:**
- Consumes: `store.Migrate` (Task 6), `requireTestDB` (Task 6, same package).
- Produces: `store.PodMetricRow{Time time.Time; Namespace, Pod, Status string; CPU, Memory float64; RestartCount int32}`, `store.PodSummary{Namespace, Pod, Status string; RestartCount int32; CPU, Memory float64; Time time.Time}`, `store.Anomaly{Time time.Time; Namespace, Pod, Metric string; Value, Baseline float64; Severity string}`, `store.Store`, `store.Open(ctx context.Context, dsn string) (*Store, error)`, `(*Store).Close()`, `(*Store).InsertPodMetric(ctx, PodMetricRow) error`, `(*Store).ListPods(ctx) ([]PodSummary, error)`, `(*Store).GetPodMetrics(ctx, namespace, pod string) ([]PodMetricRow, error)`, `(*Store).ListAnomalies(ctx, namespace, pod string) ([]Anomaly, error)`.

- [ ] **Step 1: Write the failing tests**

Create `go/internal/store/store_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := requireTestDB(t)

	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func cleanNamespace(t *testing.T, s *Store, namespace string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `DELETE FROM pod_metrics WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("cleaning pod_metrics for %q: %v", namespace, err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM anomalies WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("cleaning anomalies for %q: %v", namespace, err)
	}
}

func TestStore_InsertPodMetricAndListPods(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-list-pods"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	older := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)
	newer := time.Now().UTC().Truncate(time.Millisecond)

	if err := s.InsertPodMetric(ctx, PodMetricRow{
		Time: older, Namespace: namespace, Pod: "web-1",
		CPU: 0.1, Memory: 1e8, Status: "Running", RestartCount: 0,
	}); err != nil {
		t.Fatalf("InsertPodMetric() error = %v", err)
	}
	if err := s.InsertPodMetric(ctx, PodMetricRow{
		Time: newer, Namespace: namespace, Pod: "web-1",
		CPU: 0.2, Memory: 2e8, Status: "Running", RestartCount: 1,
	}); err != nil {
		t.Fatalf("InsertPodMetric() error = %v", err)
	}

	pods, err := s.ListPods(ctx)
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}

	var found *PodSummary
	for i := range pods {
		if pods[i].Namespace == namespace && pods[i].Pod == "web-1" {
			found = &pods[i]
		}
	}
	if found == nil {
		t.Fatalf("ListPods() did not include %s/web-1", namespace)
	}
	if found.RestartCount != 1 || found.CPU != 0.2 {
		t.Errorf("ListPods() returned stale row: %+v, want the latest sample (restart_count=1, cpu=0.2)", found)
	}
}

func TestStore_GetPodMetricsReturnsTimeSeriesOldestFirst(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-get-metrics"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	t1 := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Millisecond)
	t2 := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)

	for _, row := range []PodMetricRow{
		{Time: t2, Namespace: namespace, Pod: "api-1", CPU: 0.2, Memory: 2e8, Status: "Running"},
		{Time: t1, Namespace: namespace, Pod: "api-1", CPU: 0.1, Memory: 1e8, Status: "Running"},
	} {
		if err := s.InsertPodMetric(ctx, row); err != nil {
			t.Fatalf("InsertPodMetric() error = %v", err)
		}
	}

	samples, err := s.GetPodMetrics(ctx, namespace, "api-1")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2", len(samples))
	}
	if !samples[0].Time.Equal(t1) || !samples[1].Time.Equal(t2) {
		t.Errorf("samples not ordered oldest-first: %+v", samples)
	}
}

func TestStore_ListAnomaliesReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-anomalies-empty"
	cleanNamespace(t, s, namespace)

	anomalies, err := s.ListAnomalies(context.Background(), namespace, "unknown-pod")
	if err != nil {
		t.Fatalf("ListAnomalies() error = %v", err)
	}
	if anomalies == nil {
		t.Error("ListAnomalies() = nil, want empty non-nil slice")
	}
	if len(anomalies) != 0 {
		t.Errorf("len(anomalies) = %d, want 0", len(anomalies))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/store/...`
Expected: FAIL — compile error, `Store`/`Open`/`PodMetricRow` etc. undefined.

- [ ] **Step 3: Implement the store**

Create `go/internal/store/store.go`:

```go
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PodMetricRow is one poll cycle's persisted sample for a pod.
type PodMetricRow struct {
	Time         time.Time
	Namespace    string
	Pod          string
	CPU          float64
	Memory       float64
	Status       string
	RestartCount int32
}

// PodSummary is the latest known row for one pod — the dashboard's pod
// list view.
type PodSummary struct {
	Namespace    string
	Pod          string
	Status       string
	RestartCount int32
	CPU          float64
	Memory       float64
	Time         time.Time
}

// Anomaly is a single detected deviation for a pod's metric.
type Anomaly struct {
	Time      time.Time
	Namespace string
	Pod       string
	Metric    string
	Value     float64
	Baseline  float64
	Severity  string
}

// Store persists and queries pod metrics and anomalies in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and applies pending schema migrations.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if err := Migrate(dsn); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// InsertPodMetric records one pod's sample for a poll cycle.
func (s *Store) InsertPodMetric(ctx context.Context, row PodMetricRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pod_metrics (time, namespace, pod, cpu, memory, status, restart_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, row.Time, row.Namespace, row.Pod, row.CPU, row.Memory, row.Status, row.RestartCount)
	if err != nil {
		return fmt.Errorf("inserting pod metric for %s/%s: %w", row.Namespace, row.Pod, err)
	}
	return nil
}

// ListPods returns the latest known row for every pod that has ever
// reported a metric.
func (s *Store) ListPods(ctx context.Context) ([]PodSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (namespace, pod)
			namespace, pod, status, restart_count, cpu, memory, time
		FROM pod_metrics
		ORDER BY namespace, pod, time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	defer rows.Close()

	var summaries []PodSummary
	for rows.Next() {
		var summary PodSummary
		if err := rows.Scan(&summary.Namespace, &summary.Pod, &summary.Status, &summary.RestartCount, &summary.CPU, &summary.Memory, &summary.Time); err != nil {
			return nil, fmt.Errorf("scanning pod summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// GetPodMetrics returns a pod's metric samples from the last hour,
// oldest first.
func (s *Store) GetPodMetrics(ctx context.Context, namespace, pod string) ([]PodMetricRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, namespace, pod, cpu, memory, status, restart_count
		FROM pod_metrics
		WHERE namespace = $1 AND pod = $2 AND time >= now() - interval '1 hour'
		ORDER BY time ASC
	`, namespace, pod)
	if err != nil {
		return nil, fmt.Errorf("getting metrics for %s/%s: %w", namespace, pod, err)
	}
	defer rows.Close()

	var samples []PodMetricRow
	for rows.Next() {
		var r PodMetricRow
		if err := rows.Scan(&r.Time, &r.Namespace, &r.Pod, &r.CPU, &r.Memory, &r.Status, &r.RestartCount); err != nil {
			return nil, fmt.Errorf("scanning pod metric row: %w", err)
		}
		samples = append(samples, r)
	}
	return samples, rows.Err()
}

// ListAnomalies returns a pod's anomaly history, most recent first. It
// returns an empty slice (not an error) until the Python anomaly
// detector exists and starts writing to the anomalies table.
func (s *Store) ListAnomalies(ctx context.Context, namespace, pod string) ([]Anomaly, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, namespace, pod, metric, value, baseline, severity
		FROM anomalies
		WHERE namespace = $1 AND pod = $2
		ORDER BY time DESC
	`, namespace, pod)
	if err != nil {
		return nil, fmt.Errorf("listing anomalies for %s/%s: %w", namespace, pod, err)
	}
	defer rows.Close()

	anomalies := []Anomaly{}
	for rows.Next() {
		var a Anomaly
		if err := rows.Scan(&a.Time, &a.Namespace, &a.Pod, &a.Metric, &a.Value, &a.Baseline, &a.Severity); err != nil {
			return nil, fmt.Errorf("scanning anomaly row: %w", err)
		}
		anomalies = append(anomalies, a)
	}
	return anomalies, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/store/...`
Expected: PASS (4 tests total).

- [ ] **Step 5: Commit**

```bash
cd go
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(go-agent): add postgres store CRUD methods"
```

---

## Task 8: Poll-and-persist orchestration

**Files:**
- Create: `go/internal/ingest/ingest.go`
- Test: `go/internal/ingest/ingest_test.go`

**Interfaces:**
- Consumes: `k8s.PodSample` (Task 4), `store.PodMetricRow` (Task 7).
- Produces: `ingest.Poller` (interface: `Poll(ctx) []k8s.PodSample`), `ingest.Store` (interface: `InsertPodMetric(ctx, store.PodMetricRow) error`), `ingest.Run(ctx context.Context, poller Poller, st Store, logger *slog.Logger)`. `*k8s.Poller` and `*store.Store` both satisfy these interfaces without changes.

- [ ] **Step 1: Write the failing tests**

Create `go/internal/ingest/ingest_test.go`:

```go
package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

type fakePoller struct {
	samples []k8s.PodSample
}

func (f fakePoller) Poll(ctx context.Context) []k8s.PodSample {
	return f.samples
}

type fakeStore struct {
	inserted []store.PodMetricRow
	failFor  string // pod name to fail insert for
}

func (f *fakeStore) InsertPodMetric(ctx context.Context, row store.PodMetricRow) error {
	if row.Pod == f.failFor {
		return errors.New("simulated insert failure")
	}
	f.inserted = append(f.inserted, row)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRun_PersistsEverySample(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "web-1", Status: "Running", CPU: 0.1, Memory: 1e8, Timestamp: now},
		{Namespace: "default", Name: "web-2", Status: "Running", CPU: 0.2, Memory: 2e8, Timestamp: now},
	}}
	st := &fakeStore{}

	Run(context.Background(), poller, st, testLogger())

	if len(st.inserted) != 2 {
		t.Fatalf("len(inserted) = %d, want 2", len(st.inserted))
	}
}

func TestRun_SkipsFailedInsertWithoutStoppingTheRest(t *testing.T) {
	now := time.Now()
	poller := fakePoller{samples: []k8s.PodSample{
		{Namespace: "default", Name: "bad-pod", Status: "Running", Timestamp: now},
		{Namespace: "default", Name: "good-pod", Status: "Running", Timestamp: now},
	}}
	st := &fakeStore{failFor: "bad-pod"}

	Run(context.Background(), poller, st, testLogger())

	if len(st.inserted) != 1 || st.inserted[0].Pod != "good-pod" {
		t.Fatalf("inserted = %+v, want only good-pod to have been persisted", st.inserted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/ingest/...`
Expected: FAIL — compile error, package `ingest` / `Run` undefined.

- [ ] **Step 3: Implement the orchestration**

Create `go/internal/ingest/ingest.go`:

```go
// Package ingest wires the Kubernetes poller to the Postgres store,
// running one poll-and-persist cycle at a time.
package ingest

import (
	"context"
	"log/slog"

	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

// Poller is the subset of *k8s.Poller that ingest depends on.
type Poller interface {
	Poll(ctx context.Context) []k8s.PodSample
}

// Store is the subset of *store.Store that ingest depends on.
type Store interface {
	InsertPodMetric(ctx context.Context, row store.PodMetricRow) error
}

// Run polls once and persists every sample it gets back. A failed
// insert for one pod is logged and skipped, not fatal to the rest of
// the cycle — matches the fault isolation used for polling itself.
func Run(ctx context.Context, poller Poller, st Store, logger *slog.Logger) {
	samples := poller.Poll(ctx)

	for _, sample := range samples {
		row := store.PodMetricRow{
			Time:         sample.Timestamp,
			Namespace:    sample.Namespace,
			Pod:          sample.Name,
			CPU:          sample.CPU,
			Memory:       sample.Memory,
			Status:       sample.Status,
			RestartCount: sample.RestartCount,
		}

		if err := st.InsertPodMetric(ctx, row); err != nil {
			logger.Error("persisting pod metric failed, skipping", "namespace", sample.Namespace, "pod", sample.Name, "error", err)
			continue
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/ingest/...`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
cd go
git add internal/ingest/ingest.go internal/ingest/ingest_test.go
git commit -m "feat(go-agent): orchestrate poll-and-persist with per-pod fault isolation"
```

---

## Task 9: Wire REST handlers to the store

**Files:**
- Modify: `go/internal/api/handlers.go`
- Modify: `go/internal/api/router.go`
- Test: `go/internal/api/handlers_test.go` (new)
- Modify: `docs/kube-agent.md`

**Interfaces:**
- Consumes: `store.Store` and its methods (Task 7), `models.PodSummary`/`models.PodDetail`/`models.MetricSample`/`models.Anomaly` (existing, unchanged).
- Produces: `api.NewRouter(logger *slog.Logger, st *store.Store) http.Handler` (signature change — was `NewRouter(logger *slog.Logger)`).

- [ ] **Step 1: Write the failing tests**

Create `go/internal/api/handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"podsentinel/internal/models"
	"podsentinel/internal/store"
)

func testDSN() string {
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable"
}

func newTestRouter(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	dsn := testDSN()

	st, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping: Postgres not reachable at %s (is `docker compose -f infra/docker-compose.yml up -d` running?): %v", dsn, err)
	}
	t.Cleanup(st.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, st), st, dsn
}

func cleanupNamespace(t *testing.T, dsn, namespace string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return
	}
	defer pool.Close()
	_, _ = pool.Exec(context.Background(), `DELETE FROM pod_metrics WHERE namespace = $1`, namespace)
}

func TestListPods_ReturnsStoreData(t *testing.T) {
	router, st, dsn := newTestRouter(t)
	namespace := "api-test-list-pods"
	t.Cleanup(func() { cleanupNamespace(t, dsn, namespace) })

	ctx := context.Background()
	if err := st.InsertPodMetric(ctx, store.PodMetricRow{
		Time: time.Now().UTC().Truncate(time.Millisecond), Namespace: namespace, Pod: "web-1",
		CPU: 0.1, Memory: 1e8, Status: "Running", RestartCount: 2,
	}); err != nil {
		t.Fatalf("seeding pod metric: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var pods []models.PodSummary
	if err := json.NewDecoder(rec.Body).Decode(&pods); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	var found bool
	for _, p := range pods {
		if p.Namespace == namespace && p.Name == "web-1" && p.RestartCount == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("response %+v did not include the seeded pod", pods)
	}
}

func TestListPodAnomalies_ReturnsEmptyArrayNotNull(t *testing.T) {
	router, _, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/no-anomalies-yet/anomalies", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want %q (empty array, not null)", body, "[]\n")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `go/`): `go test ./internal/api/...`
Expected: FAIL — compile error, `NewRouter` called with 2 args but only takes 1.

- [ ] **Step 3: Rewrite the handlers**

Replace `go/internal/api/handlers.go` with:

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"podsentinel/internal/models"
	"podsentinel/internal/store"
)

// handlers holds the dependencies shared by the API's HTTP handlers.
type handlers struct {
	logger *slog.Logger
	store  *store.Store
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) listPods(w http.ResponseWriter, r *http.Request) {
	pods, err := h.store.ListPods(r.Context())
	if err != nil {
		h.logger.Error("listing pods failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list pods"})
		return
	}

	summaries := make([]models.PodSummary, 0, len(pods))
	for _, p := range pods {
		summaries = append(summaries, models.PodSummary{
			Namespace:    p.Namespace,
			Name:         p.Pod,
			Status:       p.Status,
			RestartCount: int(p.RestartCount),
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *handlers) getPod(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.GetPodMetrics(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("getting pod metrics failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get pod metrics"})
		return
	}

	detail := models.PodDetail{
		Namespace: namespace,
		Name:      pod,
		Metrics:   make([]models.MetricSample, 0, len(rows)),
	}
	for i, row := range rows {
		if i == len(rows)-1 {
			detail.Status = row.Status
			detail.RestartCount = int(row.RestartCount)
		}
		detail.Metrics = append(detail.Metrics, models.MetricSample{
			Timestamp: row.Time,
			CPU:       row.CPU,
			Memory:    row.Memory,
		})
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) listPodAnomalies(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.ListAnomalies(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("listing anomalies failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list anomalies"})
		return
	}

	anomalies := make([]models.Anomaly, 0, len(rows))
	for _, a := range rows {
		anomalies = append(anomalies, models.Anomaly{
			Timestamp: a.Time,
			Namespace: a.Namespace,
			Pod:       a.Pod,
			Metric:    a.Metric,
			Value:     a.Value,
			Baseline:  a.Baseline,
			Severity:  a.Severity,
		})
	}
	writeJSON(w, http.StatusOK, anomalies)
}
```

- [ ] **Step 4: Update the router**

Replace `go/internal/api/router.go` with:

```go
// Package api exposes the Go agent's REST API: a chi router serving
// the endpoints the dashboard reads from, backed by the Postgres
// store (see docs/architecture.md for the underlying schema).
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"podsentinel/internal/store"
)

// NewRouter builds the agent's HTTP handler tree.
func NewRouter(logger *slog.Logger, st *store.Store) http.Handler {
	h := &handlers{logger: logger, store: st}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/health", h.health)

	r.Route("/api/v1/pods", func(r chi.Router) {
		r.Get("/", h.listPods)
		r.Get("/{namespace}/{pod}", h.getPod)
		r.Get("/{namespace}/{pod}/anomalies", h.listPodAnomalies)
	})

	return r
}

// requestLogger logs each request's method, path, status, and duration
// through the agent's structured logger.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration", time.Since(start).String(),
			)
		})
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run (from `go/`): `go test ./internal/api/...`
Expected: PASS. If it instead reports "skipping: Postgres not reachable", run `docker compose -f infra/docker-compose.yml up -d` from the repo root first.

- [ ] **Step 6: Update `docs/kube-agent.md`**

Change the "Current stage" line near the top from:

```markdown
**Current stage**: scaffold only — REST API endpoints return
placeholder data. Kubernetes Metrics API polling, Postgres writes, and
RabbitMQ pub/sub are follow-up work (see "Next steps" below).
```

to:

```markdown
**Current stage**: core ingestion path implemented — the agent polls
Kubernetes (pod status/restarts + CPU/memory usage) on an interval and
writes every sample to Postgres/TimescaleDB; REST API endpoints read
real data. RabbitMQ pub/sub and Python anomaly detection are follow-up
work (see "Next steps" below).
```

Update the `## Configuration` table to add the three new env vars (after the `LOG_LEVEL` row):

```markdown
| `POSTGRES_DSN` | *(none — required)* | Postgres/TimescaleDB connection string |
| `WATCH_NAMESPACES` | *(empty = all namespaces)* | Comma-separated namespace allow-list |
| `POLL_INTERVAL` | `15s` | How often to poll the Kubernetes APIs |
```

Change the line right below "## Endpoint reference"'s table from:

```markdown
All responses are placeholder data today — response shapes are defined
in `go/internal/models`.
```

to:

```markdown
Response shapes are defined in `go/internal/models`; `/api/v1/pods/{namespace}/{pod}/anomalies`
returns an empty array until the Python anomaly detector (a later
build-order step) exists to write to it.
```

Update `## Packages` to add the three new packages (after the `internal/models` line):

```markdown
- `internal/k8s` — builds Kubernetes clients (in-cluster or kubeconfig fallback), polls pods + metrics, and joins them per cycle.
- `internal/store` — Postgres/TimescaleDB schema migrations and CRUD queries.
- `internal/ingest` — orchestrates one poll-and-persist cycle, with per-pod fault isolation.
```

Update `## Next steps` from:

```markdown
Tracked as follow-up work, not yet implemented:

- Kubernetes Metrics API polling via `client-go`.
- Postgres/TimescaleDB connection and writes.
- RabbitMQ publishing (`metrics.raw`) and consuming (`anomalies.detected`).
- Deploying into the local KIND cluster (see [`infra/kind.md`](infra/kind.md)) once the agent has something real to poll and `metrics-server` is installed.
```

to:

```markdown
Tracked as follow-up work, not yet implemented:

- RabbitMQ publishing (`metrics.raw`) and consuming (`anomalies.detected`).
- Python anomaly detection, which will populate the (currently empty) `anomalies` table.
- Deploying the agent itself into the local KIND cluster (RBAC ServiceAccount/ClusterRole for pod + `metrics.k8s.io` access, a Deployment manifest) — today it runs on the host against KIND via kubeconfig.
```

- [ ] **Step 7: Commit (two commits — code, then docs)**

```bash
git add go/internal/api/handlers.go go/internal/api/router.go go/internal/api/handlers_test.go
git commit -m "feat(go-agent): serve REST API from the postgres store"

git add docs/kube-agent.md
git commit -m "docs(go-agent): document real ingestion path in kube-agent.md"
```

---

## Task 10: Wire everything into `main.go`

**Files:**
- Modify: `go/cmd/agent/main.go`

**Interfaces:**
- Consumes: `k8s.BuildClients` (Task 3), `k8s.NewPoller`/`*k8s.Poller` (Task 5), `store.Open`/`*store.Store` (Task 7), `ingest.Run` (Task 8), `api.NewRouter` (Task 9), `config.Config.WatchNamespaces`/`.PollInterval`/`.PostgresDSN` (Task 2).
- Produces: the runnable `cmd/agent` binary — no further tasks depend on `main.go`'s internals.

- [ ] **Step 1: Replace `go/cmd/agent/main.go`**

```go
// Command agent runs the PodSentinel Go agent: it polls the
// Kubernetes Metrics API, writes to Postgres, and serves the REST API
// the dashboard reads from. RabbitMQ pub/sub is follow-up work.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"podsentinel/internal/api"
	"podsentinel/internal/config"
	"podsentinel/internal/ingest"
	"podsentinel/internal/k8s"
	"podsentinel/internal/logging"
	"podsentinel/internal/store"
)

func main() {
	cfg := config.Load()
	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clients, err := k8s.BuildClients()
	if err != nil {
		logger.Error("building kubernetes clients failed", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("connecting to postgres failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	poller := k8s.NewPoller(clients, cfg.WatchNamespaces, logger)

	go runPollLoop(ctx, poller, st, logger, cfg.PollInterval)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: api.NewRouter(logger, st),
	}

	go func() {
		logger.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}

// runPollLoop runs one poll-and-persist cycle immediately, then again
// on every tick of interval, until ctx is cancelled.
func runPollLoop(ctx context.Context, poller *k8s.Poller, st *store.Store, logger *slog.Logger, interval time.Duration) {
	ingest.Run(ctx, poller, st, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ingest.Run(ctx, poller, st, logger)
		}
	}
}
```

- [ ] **Step 2: Verify the full build and test suite**

Run (from `go/`): `go build ./... && go vet ./... && go test ./...`
Expected: build and vet succeed with no output; all tests PASS (skipping cleanly if Postgres isn't reachable).

- [ ] **Step 3: Manual smoke test**

Ensure both are running first: `kubectl config current-context` reports `kind-podsentinel`, and `docker compose -f infra/docker-compose.yml ps` shows `timescaledb` as `Up`.

```bash
cd go
POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable' go run ./cmd/agent
```

In another terminal:
```bash
curl localhost:8080/health
curl localhost:8080/api/v1/pods
```
Expected: `/health` returns `{"status":"ok"}`; `/api/v1/pods` returns real pods from the KIND cluster (not the old placeholder `example-pod`) once the first poll cycle completes (within `POLL_INTERVAL`, default 15s).

- [ ] **Step 4: Commit**

```bash
cd go
git add cmd/agent/main.go
git commit -m "feat(go-agent): wire poller, store, and REST API together in main"
```

---

## Task 11: KIND + docker-compose integration test

**Files:**
- Create: `go/internal/integration/agent_test.go`
- Modify: `docs/kube-agent.md`

**Interfaces:**
- Consumes: `k8s.BuildClients`/`k8s.NewPoller` (Tasks 3, 5), `store.Open` (Task 7), `ingest.Run` (Task 8).
- Produces: nothing further downstream — this is the top of the dependency chain.

- [ ] **Step 1: Write the integration test**

Create `go/internal/integration/agent_test.go`:

```go
//go:build integration

// Package integration exercises the Go agent's real ingestion path
// against a live KIND cluster and the docker-compose TimescaleDB
// instance. Run with:
//
//	go test -tags=integration ./internal/integration/...
//
// Requires: the kind-podsentinel cluster (see docs/infra/kind.md, with
// metrics-server installed) as the current kubectl context, and
// infra/docker-compose.yml's timescaledb service running.
package integration

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"podsentinel/internal/ingest"
	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAgent_PollsRealClusterAndPersistsToRealPostgres(t *testing.T) {
	ctx := context.Background()

	clients, err := k8s.BuildClients()
	if err != nil {
		t.Fatalf("k8s.BuildClients() error = %v (is the kind-podsentinel cluster reachable?)", err)
	}

	namespace := "podsentinel-integration-test"
	if _, err := clients.Core.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating test namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = clients.Core.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})

	podName := "ingestion-path-probe"
	if _, err := clients.Core.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "probe",
				Image: "registry.k8s.io/pause:3.9",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
				},
			}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating test pod: %v", err)
	}

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable"
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("store.Open() error = %v (is `docker compose -f infra/docker-compose.yml up -d` running?)", err)
	}
	t.Cleanup(st.Close)

	poller := k8s.NewPoller(clients, []string{namespace}, testLogger())

	deadline := time.Now().Add(90 * time.Second)
	var seen bool
	for time.Now().Before(deadline) {
		ingest.Run(ctx, poller, st, testLogger())

		pods, err := st.ListPods(ctx)
		if err != nil {
			t.Fatalf("ListPods() error = %v", err)
		}
		for _, p := range pods {
			if p.Namespace == namespace && p.Pod == podName {
				seen = true
			}
		}
		if seen {
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !seen {
		t.Fatalf("pod %s/%s never appeared via the real ingestion path within 90s (metrics-server may need longer to scrape a brand-new pod)", namespace, podName)
	}
}
```

- [ ] **Step 2: Run the integration test**

Ensure both are running: `kubectl config current-context` reports `kind-podsentinel`, and `docker compose -f infra/docker-compose.yml ps` shows `timescaledb` as `Up`.

Run (from `go/`): `go test -tags=integration ./internal/integration/...`
Expected: PASS within ~90s. (`go test ./...` without `-tags=integration` must still skip this package cleanly — the build tag excludes it entirely from the default build list.)

- [ ] **Step 3: Update `docs/kube-agent.md` with the integration test**

Add a new section after `## Verify`:

```markdown
## Integration test

`go test -tags=integration ./internal/integration/...` exercises the
full ingestion path against real infrastructure: it creates a
namespace and pod in the `kind-podsentinel` cluster, runs poll cycles
against `infra/docker-compose.yml`'s TimescaleDB until the pod shows up
via `store.ListPods`, then cleans up. Requires both to already be
running (see [`infra/kind.md`](infra/kind.md) and
[`infra/docker-compose.md`](infra/docker-compose.md)) — it is not part
of `go test ./...` and is not CI-portable as written.
```

- [ ] **Step 4: Commit (two commits — test, then docs)**

```bash
git add go/internal/integration/agent_test.go
git commit -m "test(go-agent): add kind+compose integration test for the ingestion path"

git add docs/kube-agent.md
git commit -m "docs(go-agent): document how to run the integration test"
```

---

## Final verification

- [ ] Run (from `go/`): `go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [ ] Expected: everything passes. `go test ./...` should show no skips if `infra/docker-compose.yml` is up (as it already is); `go test -tags=integration ./...` should show no skips if both `infra/docker-compose.yml` and `kind-podsentinel` are up (as they already are).
- [ ] Manual: `go run ./cmd/agent`, then create a pod in a namespace covered by `WATCH_NAMESPACES` (or leave it unset to watch all) and confirm it appears at `curl localhost:8080/api/v1/pods` within one `POLL_INTERVAL`; confirm a pod in an excluded namespace never appears.
