// Package testdb creates an isolated PostgreSQL database for each test.
// Tests that use it need NEXFLOW_TEST_DB_URL, which points at a PostgreSQL
// server where the test user may create databases. Without it, tests skip
// with a clear message; they never pass silently.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvVar names the environment variable holding the test server URL.
const EnvVar = "NEXFLOW_TEST_DB_URL"

// migrateFn applies the schema to a fresh database. Set by package migrations
// wiring once migrations exist; empty until then.
var migrateFn func(ctx context.Context, pool *pgxpool.Pool) error

// SetMigrator wires the schema migration used by New. Called from test setup
// in packages that need the schema.
func SetMigrator(fn func(ctx context.Context, pool *pgxpool.Pool) error) {
	migrateFn = fn
}

// New returns a pool connected to a new, empty database with the schema applied.
// The database is dropped when the test ends.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	serverURL := os.Getenv(EnvVar)
	if serverURL == "" {
		t.Skipf("%s is not set: skipping database test (set it to run database tests)", EnvVar)
	}

	ctx := context.Background()
	name := newName(t)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("testdb: connect to server: %v", err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q", name)); err != nil {
		admin.Close(ctx)
		t.Fatalf("testdb: create database %s: %v", name, err)
	}

	cfg, err := pgxpool.ParseConfig(serverURL)
	if err != nil {
		admin.Close(ctx)
		t.Fatalf("testdb: parse url: %v", err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close(ctx)
		t.Fatalf("testdb: open pool: %v", err)
	}

	if migrateFn != nil {
		if err := migrateFn(ctx, pool); err != nil {
			pool.Close()
			admin.Close(ctx)
			t.Fatalf("testdb: migrate: %v", err)
		}
	}

	t.Cleanup(func() {
		pool.Close()
		cctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := admin.Exec(cctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q WITH (FORCE)", name)); err != nil {
			t.Errorf("testdb: drop database %s: %v", name, err)
		}
		admin.Close(cctx)
	})

	return pool
}

func newName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("testdb: random name: %v", err)
	}
	return "nf_test_" + hex.EncodeToString(b)
}
