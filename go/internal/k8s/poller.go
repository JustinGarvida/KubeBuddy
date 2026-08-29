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
