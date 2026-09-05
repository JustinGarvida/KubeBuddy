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
