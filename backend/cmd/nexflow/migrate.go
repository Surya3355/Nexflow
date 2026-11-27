package main

import (
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrateUp applies the embedded schema migrations.
func migrateUp(pool *pgxpool.Pool) error {
	return migrations.Up(pool)
}
