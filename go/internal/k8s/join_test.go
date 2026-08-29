package k8s

import (
	"io"
	"log/slog"
	"math"
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
	if math.Abs(got.CPU-0.15) > 1e-10 {
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
