package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

// appPool opens a second pool on the same database that acts as the app role.
func appPool(t *testing.T, owner *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := owner.Config().Copy()
	UseAppRole(cfg)
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestAppRoleCannotChangeAudit(t *testing.T) {
	owner := testdb.New(t)
	if err := Up(owner); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := appPool(t, owner)
	ctx := context.Background()
	var who string
	mustScan(t, app.QueryRow(ctx, `SELECT current_user`), &who)
	if who != "nexflow_app" {
		t.Fatalf("app pool acts as %q, want nexflow_app", who)
	}
	if _, err := app.Exec(ctx, `INSERT INTO audit_log (action, entity_type) VALUES ('test.added', 'test')`); err != nil {
		t.Fatalf("app can't add history: %v", err)
	}
	for _, sql := range []string{
		`UPDATE audit_log SET action = 'changed'`,
		`DELETE FROM audit_log`,
		`TRUNCATE audit_log`,
	} {
		_, err := app.Exec(ctx, sql)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s: err = %v, want permission denied", sql, err)
		}
	}
}

func TestAppRoleCanUseEverythingElse(t *testing.T) {
	owner := testdb.New(t)
	if err := Up(owner); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := appPool(t, owner)
	ctx := context.Background()
	var id int64
	mustScan(t, app.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('App', '9123456789') RETURNING id`), &id)
	if _, err := app.Exec(ctx, `UPDATE customers SET name = 'App 2' WHERE id = $1`, id); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := app.Exec(ctx, `DELETE FROM customers WHERE id = $1`, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := app.Exec(ctx, `INSERT INTO sessions (token, data, expiry) VALUES ('t', '\x00', now())`); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	// Every table except the audit log allows all four changes.
	var missing []string
	rows, err := owner.Query(ctx, `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname NOT IN ('audit_log', 'schema_migrations')
		  AND NOT (has_table_privilege('nexflow_app', c.oid, 'SELECT') AND has_table_privilege('nexflow_app', c.oid, 'INSERT')
		       AND has_table_privilege('nexflow_app', c.oid, 'UPDATE') AND has_table_privilege('nexflow_app', c.oid, 'DELETE'))`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		missing = append(missing, n)
	}
	if len(missing) > 0 {
		t.Fatalf("app role lacks rights on %v", missing)
	}
}

func TestAppRoleOnExistingDB(t *testing.T) {
	owner := testdb.New(t)
	if err := migrateTo(owner, 11); err != nil {
		t.Fatalf("migrate to 11: %v", err)
	}
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `INSERT INTO customers (name, mobile) VALUES ('Old', '9000000123')`); err != nil {
		t.Fatal(err)
	}
	if err := Up(owner); err != nil {
		t.Fatalf("migrate up on existing data: %v", err)
	}
	app := appPool(t, owner)
	var n int
	mustScan(t, app.QueryRow(ctx, `SELECT count(*) FROM customers WHERE name = 'Old'`), &n)
	if n != 1 {
		t.Fatalf("existing customer not readable by the app role")
	}
}
