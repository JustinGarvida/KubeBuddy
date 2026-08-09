// Package models defines the API's response shapes. They mirror the
// pod_metrics and anomalies tables described in docs/architecture.md,
// so real data can slot in later without changing handler signatures.
package models

import "time"

// PodSummary is a single row in the dashboard's pod list view.
type PodSummary struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	RestartCount int    `json:"restartCount"`
}

// MetricSample is a single CPU/memory reading for a pod at a point in time.
type MetricSample struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
}

// PodDetail is the dashboard's pod detail view: identity plus recent metrics.
type PodDetail struct {
	Namespace    string         `json:"namespace"`
	Name         string         `json:"name"`
	Status       string         `json:"status"`
	RestartCount int            `json:"restartCount"`
	Metrics      []MetricSample `json:"metrics"`
}

// Anomaly is a single detected deviation for a pod's metric.
type Anomaly struct {
	Timestamp time.Time `json:"timestamp"`
	Namespace string    `json:"namespace"`
	Pod       string    `json:"pod"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Baseline  float64   `json:"baseline"`
	Severity  string    `json:"severity"`
}
