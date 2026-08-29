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
