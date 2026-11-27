package history

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestAuditRowWritten(t *testing.T) {
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rec := NewPgRecorder(pool)
	id := int64(7)
	if err := rec.Record(context.Background(), Entry{
		Action: "customer.created", EntityType: "customer", EntityID: &id,
		Details: map[string]string{"mobile_last4": "3210"},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	var action, details string
	if err := pool.QueryRow(context.Background(),
		`SELECT action, details::text FROM audit_log WHERE entity_id = 7`).Scan(&action, &details); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if action != "customer.created" || details == "" {
		t.Fatalf("audit row = %q %q", action, details)
	}
}

func TestApplicationRoleCannotUpdateOrDeleteAudit(t *testing.T) {
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	if err := NewPgRecorder(pool).Record(ctx, Entry{Action: "x", EntityType: "y"}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	for _, stmt := range []string{
		`UPDATE audit_log SET action = 'tampered'`,
		`DELETE FROM audit_log`,
	} {
		err := runAsAppRole(ctx, pool, stmt)
		if !isPermissionDenied(err) {
			t.Fatalf("%q as nexflow_app: err = %v, want permission denied", stmt, err)
		}
	}
	// The same role can still insert and read.
	if err := runAsAppRole(ctx, pool, `INSERT INTO audit_log (action, entity_type) VALUES ('ok', 'y')`); err != nil {
		t.Fatalf("insert as nexflow_app: %v", err)
	}
}

func runAsAppRole(ctx context.Context, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, stmt string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE nexflow_app`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, stmt)
	return err
}

func isPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}

func TestAuditRollsBackWithTransaction(t *testing.T) {
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := NewPgRecorder(pool).Record(WithTx(ctx, tx), Entry{Action: "rolled.back", EntityType: "y"}); err != nil {
		t.Fatalf("Record in tx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'rolled.back'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("audit row survived rollback: count=%d", n)
	}
}
