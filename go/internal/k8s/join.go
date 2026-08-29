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
// no matching metrics entry — e.g. Pending, CrashLoopBackOff, Evicted,
// Failed, or a completed Job pod, all of which metrics-server never
// reports on — still gets a PodSample with CPU/Memory zeroed, since its
// status/restart count come from the core API and are always available
// regardless of metrics. This is logged at Debug: it's an expected,
// routine case, not a fault.
func joinPodsAndMetrics(pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics, now time.Time, logger *slog.Logger) []PodSample {
	metricsByKey := make(map[string]metricsv1beta1.PodMetrics, len(metrics))
	for _, m := range metrics {
		metricsByKey[m.Namespace+"/"+m.Name] = m
	}

	samples := make([]PodSample, 0, len(pods))
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		m, ok := metricsByKey[key]

		var cpu, memory float64
		timestamp := now
		if ok {
			for _, c := range m.Containers {
				cpu += c.Usage.Cpu().AsApproximateFloat64()
				memory += c.Usage.Memory().AsApproximateFloat64()
			}
			// Prefer metrics-server's own scrape timestamp over the poll
			// wall-clock: metrics-server serves cached values on its own
			// ~60s cadence, so at the default 15s poll interval several
			// consecutive polls can really be the same underlying
			// reading. Using the scrape time (rather than "now" for
			// every poll) avoids deflating variance for the z-score/EWMA
			// anomaly detector. Fall back to now if unset.
			if !m.Timestamp.Time.IsZero() {
				timestamp = m.Timestamp.Time
			}
		} else {
			logger.Debug("no metrics for pod, recording with zeroed usage", "namespace", pod.Namespace, "pod", pod.Name)
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
			Timestamp:    timestamp,
		})
	}

	return samples
}
