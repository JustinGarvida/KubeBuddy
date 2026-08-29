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
