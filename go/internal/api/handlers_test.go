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
