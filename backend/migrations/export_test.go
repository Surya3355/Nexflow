package migrations

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrateTo moves the schema to exactly version v (test only).
func migrateTo(pool *pgxpool.Pool, v uint) error {
	m, closeFn, err := newMigrator(pool)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := m.Migrate(v); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrations: to %d: %w", v, err)
	}
	return nil
}
