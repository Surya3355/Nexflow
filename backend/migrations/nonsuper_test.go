package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAppRoleWithNonSuperuserOwner is the usual managed-hosting setup: the
// database owner is not a superuser but may create roles. On PostgreSQL 16 the
// creator of nexflow_app gets ADMIN on it but not the right to SET ROLE to it;
// this grant state is set up by hand here (the role already exists on the
// shared test server). The app must still be able to act as nexflow_app.
func TestAppRoleWithNonSuperuserOwner(t *testing.T) {
	serverURL := os.Getenv(testdb.EnvVar)
	if serverURL == "" {
		t.Skip("no test database server")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	var version int
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 160000 {
		t.Skip("needs PostgreSQL 16 grant options")
	}
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	owner, db := "nf_owner_"+hex.EncodeToString(b), "nf_test_owner_"+hex.EncodeToString(b)
	for _, sql := range []string{
		fmt.Sprintf(`CREATE ROLE %s LOGIN CREATEROLE PASSWORD 'owner-pw'`, owner),
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexflow_app') THEN CREATE ROLE nexflow_app NOLOGIN; END IF; END $$`,
		fmt.Sprintf(`GRANT nexflow_app TO %s WITH ADMIN OPTION, INHERIT FALSE, SET FALSE`, owner),
		fmt.Sprintf(`CREATE DATABASE %s OWNER %s`, db, owner),
	} {
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_, _ = admin.Exec(c, fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, db))
		_, _ = admin.Exec(c, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, owner))
	})

	cfg, err := pgxpool.ParseConfig(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User, cfg.ConnConfig.Password, cfg.ConnConfig.Database = owner, "owner-pw", db
	ownerPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()
	if err := Up(ownerPool); err != nil {
		t.Fatalf("migrate as non-superuser owner: %v", err)
	}
	appCfg := cfg.Copy()
	UseAppRole(appCfg)
	app, err := pgxpool.NewWithConfig(ctx, appCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	var who string
	if err := app.QueryRow(ctx, `SELECT current_user`).Scan(&who); err != nil {
		t.Fatalf("app connection as nexflow_app: %v", err)
	}
	if who != "nexflow_app" {
		t.Fatalf("acting as %q, want nexflow_app", who)
	}
}
