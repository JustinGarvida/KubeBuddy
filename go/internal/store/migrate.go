package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// migrationsFS embeds the schema migration SQL files at build time, so
// the agent binary needs no separate migrations directory at runtime.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies the store's schema migrations.
//
// Purpose: applies all pending schema migrations. It's safe to call
// on every agent startup — already-applied migrations are no-ops.
// Params:
//   - dsn: the Postgres connection string to migrate.
//
// Returns: nil on success (including "nothing to do"), or an error if
// the connection, driver, or a migration failed.
func Migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("opening migration connection: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("building postgres migration driver: %w", err)
	}

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "pgx", driver)
	if err != nil {
		return fmt.Errorf("building migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}
