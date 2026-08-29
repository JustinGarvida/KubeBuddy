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
