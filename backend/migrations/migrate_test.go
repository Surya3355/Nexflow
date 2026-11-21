package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaTables = []string{
	"users", "sessions", "banks", "stages", "dropdown_values",
	"customers", "cases", "case_stage_events", "case_assignment_events",
	"documents", "archive", "audit_log", "alert_log",
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name).Scan(&ok)
	if err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	return ok
}

func TestMigrateUpCreatesTables(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, name := range schemaTables {
		if !tableExists(t, pool, name) {
			t.Errorf("table %s missing after Up", name)
		}
	}
}

func TestMigrateDownThenUpIsClean(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := Down(pool); err != nil {
		t.Fatalf("Down: %v", err)
	}
	for _, name := range schemaTables {
		if tableExists(t, pool, name) {
			t.Errorf("table %s still present after Down", name)
		}
	}
	if err := Up(pool); err != nil {
		t.Fatalf("second Up: %v", err)
	}
}

func TestStandardStagesSeeded(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	rows, err := pool.Query(context.Background(),
		`SELECT code, name, bank_id IS NULL, requires_document FROM stages
		 WHERE bank_id IS NULL ORDER BY sort_order`)
	if err != nil {
		t.Fatalf("query stages: %v", err)
	}
	defer rows.Close()

	want := []string{"received:Received", "logged_in:Logged in", "under_sanction:Under sanction",
		"sanctioned:Sanctioned", "documentation:Documentation", "disbursed:Disbursed"}
	var got []string
	for rows.Next() {
		var code, name string
		var bankNull, requires bool
		if err := rows.Scan(&code, &name, &bankNull, &requires); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !bankNull {
			t.Errorf("stage %s has a bank_id, want null", code)
		}
		if requires {
			t.Errorf("stage %s requires_document = true, want false", code)
		}
		got = append(got, code+":"+name)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("standard stages = %v, want %v", got, want)
	}
}

func TestMobileCheckRejectsNonTenDigits(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO customers (name, mobile) VALUES ('Test', '+919876543210')`)
	if err == nil {
		t.Fatal("insert with +91 mobile succeeded, want check violation")
	}
}

func TestTrackingNumberUniqueAmongLiveCases(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ctx := context.Background()
	var userID, bankID, stageID, custA, custB int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('admin', 'Admin', 'x', 'admin') RETURNING id`), &userID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bankID)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`), &stageID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9876543210') RETURNING id`), &custA)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('B', '9876543211') RETURNING id`), &custB)

	insertCase := func(customer int64, tracking *string) error {
		_, err := pool.Exec(ctx, `INSERT INTO cases
			(customer_id, bank_id, loan_amount, tracking_number, current_stage_id, assignee_id)
			VALUES ($1, $2, 1000, $3, $4, $5)`, customer, bankID, tracking, stageID, userID)
		return err
	}
	tn := "12345"
	if err := insertCase(custA, &tn); err != nil {
		t.Fatalf("first case with tracking: %v", err)
	}
	if err := insertCase(custB, &tn); err == nil {
		t.Fatal("second live case with same tracking number succeeded, want unique violation")
	}
	if err := insertCase(custA, nil); err != nil {
		t.Fatalf("case without tracking: %v", err)
	}
	if err := insertCase(custB, nil); err != nil {
		t.Fatalf("second case without tracking should be allowed: %v", err)
	}
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestMigration4SetsExistingCasesToNA(t *testing.T) {
	pool := testdb.New(t)
	if err := migrateTo(pool, 3); err != nil {
		t.Fatalf("to 3: %v", err)
	}
	ctx := context.Background()
	var user, bank, customer, caseID int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role) VALUES ('u', 'u', 'x', 'admin') RETURNING id`), &user)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9000000001') RETURNING id`), &customer)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3) RETURNING id`,
		customer, bank, user), &caseID)

	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	var name string
	var system bool
	mustScan(t, pool.QueryRow(ctx, `SELECT p.name, p.is_system FROM cases c JOIN projects p ON p.id = c.project_id WHERE c.id = $1`, caseID), &name, &system)
	if name != "NA" || !system {
		t.Fatalf("existing case project = %q (system %v), want NA", name, system)
	}
	if _, err := pool.Exec(ctx, `UPDATE cases SET project_id = NULL WHERE id = $1`, caseID); err == nil {
		t.Fatal("project_id accepted NULL, want NOT NULL")
	}
}
