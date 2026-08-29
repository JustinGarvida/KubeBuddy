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
