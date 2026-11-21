// Package migrations applies the versioned SQL schema with golang-migrate.
// Migration files are embedded in the binary, so a deployed server always
// runs the schema it was built with.
package migrations

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed *.sql
var files embed.FS

// Up applies all pending migrations. Running it when the schema is already
// current is not an error.
func Up(pool *pgxpool.Pool) error {
	m, closeFn, err := newMigrator(pool)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrations: up: %w", err)
	}
	return nil
}

// Down rolls back all migrations, leaving an empty schema.
func Down(pool *pgxpool.Pool) error {
	m, closeFn, err := newMigrator(pool)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrations: down: %w", err)
	}
	return nil
}

func newMigrator(pool *pgxpool.Pool) (*migrate.Migrate, func(), error) {
	src, err := iofs.New(files, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("migrations: source: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	drv, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrations: driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrations: init: %w", err)
	}
	closeFn := func() {
		_, _ = m.Close()
	}
	return m, closeFn, nil
}
