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
	"documents", "archive", "audit_log", "alert_log", "projects", "leads", "lead_contacts", "project_contacts",
	"case_disbursements",
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

func TestMigration5BackfillsLeadsForCaselessCustomers(t *testing.T) {
	pool := testdb.New(t)
	if err := migrateTo(pool, 4); err != nil {
		t.Fatalf("to 4: %v", err)
	}
	ctx := context.Background()
	var admin, bank, a, b, c int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role) VALUES ('adm', 'Adm', 'x', 'admin') RETURNING id`), &admin)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile, created_by) VALUES ('A', '9000000001', $1) RETURNING id`, admin), &a)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('B', '9000000002') RETURNING id`), &b)
	if _, err := pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3)`, b, bank, admin); err != nil {
		t.Fatalf("case: %v", err)
	}
	reason := int64(0)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM dropdown_values WHERE category='drop_reason' LIMIT 1`), &reason)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile, status, dropped_reason_id, dropped_at) VALUES ('C', '9000000003', 'dropped', $1, now()) RETURNING id`, reason), &c)

	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	var n int
	var project string
	var assignee int64
	var callIsToday bool
	mustScan(t, pool.QueryRow(ctx, `SELECT count(*) OVER (), p.name, l.assignee_id,
		l.next_call_date = (now() AT TIME ZONE 'Asia/Kolkata')::date
		FROM leads l JOIN projects p ON p.id = l.project_id WHERE l.customer_id = $1 AND l.status = 'open'`, a),
		&n, &project, &assignee, &callIsToday)
	if n != 1 || project != "NA" || assignee != admin || !callIsToday {
		t.Fatalf("lead for A: n=%d project=%q assignee=%d today=%v", n, project, assignee, callIsToday)
	}
	for _, id := range []int64{b, c} {
		var cnt int
		mustScan(t, pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE customer_id = $1`, id), &cnt)
		if cnt != 0 {
			t.Errorf("customer %d got %d leads, want 0", id, cnt)
		}
	}
}

func TestMigration5OneOpenLeadPerProject(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ctx := context.Background()
	var user, cust int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role) VALUES ('u', 'u', 'x', 'admin') RETURNING id`), &user)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9000000001') RETURNING id`), &cust)
	ins := `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, na_project_id(), $2)`
	if _, err := pool.Exec(ctx, ins, cust, user); err != nil {
		t.Fatalf("first lead: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, cust, user); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatalf("second open lead err = %v, want 23505", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id, status, dropped_reason_id, dropped_at)
		VALUES ($1, na_project_id(), $2, 'dropped', (SELECT id FROM dropdown_values WHERE category='drop_reason' LIMIT 1), now())`, cust, user); err != nil {
		t.Fatalf("dropped lead beside open one: %v", err)
	}
}

func TestMigration5AlertNeedsOneTarget(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ctx := context.Background()
	var user, bank, cust, caseID, lead int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role) VALUES ('u', 'u', 'x', 'admin') RETURNING id`), &user)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9000000001') RETURNING id`), &cust)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3) RETURNING id`, cust, bank, user), &caseID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, na_project_id(), $2) RETURNING id`, cust, user), &lead)
	if _, err := pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('P2')`); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO alert_log (case_id, lead_id, recipient_user_id, kind) VALUES ($1, $2, $3, 'assigned')`
	if _, err := pool.Exec(ctx, ins, nil, nil, user); err == nil {
		t.Error("alert with no target accepted")
	}
	if _, err := pool.Exec(ctx, ins, caseID, lead, user); err == nil {
		t.Error("alert with two targets accepted")
	}
	if _, err := pool.Exec(ctx, ins, nil, lead, user); err != nil {
		t.Errorf("lead alert: %v", err)
	}
	// A second lead's alert of the same kind for the same user is a different alert.
	var lead2 int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id)
		VALUES ($1, (SELECT id FROM projects WHERE name = 'P2'), $2) RETURNING id`, cust, user), &lead2)
	if _, err := pool.Exec(ctx, ins, nil, lead2, user); err != nil {
		t.Errorf("second lead alert: %v", err)
	}
}

// m6Fixture makes a user, bank, customer and two projects A and B.
func m6Fixture(t *testing.T, pool *pgxpool.Pool) (user, bank, cust, a, b int64) {
	t.Helper()
	ctx := context.Background()
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role) VALUES ('u', 'u', 'x', 'admin') RETURNING id`), &user)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9000000001') RETURNING id`), &cust)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO projects (name, type, address) VALUES ('A', 'villas', 'Road 1') RETURNING id`), &a)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO projects (name, type, address) VALUES ('B', 'plots', 'Road 2') RETURNING id`), &b)
	return
}

func TestMigration6ContactFromOtherProjectRefused(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ctx := context.Background()
	user, bank, cust, a, b := m6Fixture(t, pool)
	var onB, onA int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO project_contacts (project_id, name, phone) VALUES ($1, 'Ravi', '9876543210') RETURNING id`, b), &onB)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO project_contacts (project_id, name, phone) VALUES ($1, 'Suresh', '9876543211') RETURNING id`, a), &onA)
	ins := `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, project_id, builder_contact_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3, $4, $5)`
	if _, err := pool.Exec(ctx, ins, cust, bank, user, a, onB); err == nil || !strings.Contains(err.Error(), "23503") {
		t.Fatalf("case with other project's contact err = %v, want 23503", err)
	}
	if _, err := pool.Exec(ctx, ins, cust, bank, user, a, onA); err != nil {
		t.Fatalf("case with own contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id, builder_contact_id) VALUES ($1, $2, $3, $4)`, cust, a, user, onB); err == nil || !strings.Contains(err.Error(), "23503") {
		t.Fatalf("lead with other project's contact err = %v, want 23503", err)
	}
}

func TestMigration6ActivePhoneUnique(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ctx := context.Background()
	_, _, _, a, b := m6Fixture(t, pool)
	ins := `INSERT INTO project_contacts (project_id, name, phone) VALUES ($1, 'X', '9876543210') RETURNING id`
	var first, dummy int64
	mustScan(t, pool.QueryRow(ctx, ins, a), &first)
	if err := pool.QueryRow(ctx, ins, a).Scan(&dummy); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatalf("same active phone twice err = %v, want 23505", err)
	}
	if err := pool.QueryRow(ctx, ins, b).Scan(&dummy); err != nil {
		t.Fatalf("same phone on other project: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE project_contacts SET removed_at = now() WHERE id = $1`, first); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, ins, a).Scan(&dummy); err != nil {
		t.Fatalf("same phone after removal: %v", err)
	}
}

func TestMigration6NAHasNoType(t *testing.T) {
	pool := testdb.New(t)
	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE projects SET type = 'villas' WHERE is_system`); err == nil || !strings.Contains(err.Error(), "23514") {
		t.Fatalf("NA with type err = %v, want 23514", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO projects (name, type) VALUES ('Q', 'flats')`); err == nil || !strings.Contains(err.Error(), "23514") {
		t.Fatalf("type flats err = %v, want 23514", err)
	}
}

func TestMigration7RolesAndPayments(t *testing.T) {
	pool := testdb.New(t)
	if err := migrateTo(pool, 6); err != nil {
		t.Fatalf("to 6: %v", err)
	}
	ctx := context.Background()
	user, bank, cust, a, _ := m6Fixture(t, pool)
	var paid, open int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, project_id, status, disbursed_amount, disbursement_date)
		VALUES ($1, $2, 600000, (SELECT id FROM stages WHERE code='disbursed' AND bank_id IS NULL), $3, $4, 'completed', 500000, '2026-11-05') RETURNING id`,
		cust, bank, user, a), &paid)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, project_id)
		VALUES ($1, $2, 600000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3, $4) RETURNING id`,
		cust, bank, user, a), &open)

	if err := Up(pool); err != nil {
		t.Fatalf("Up: %v", err)
	}
	roles := map[string]string{"received": "start", "logged_in": "tracking", "sanctioned": "sanction", "disbursed": "disbursement"}
	for code, want := range roles {
		var got string
		mustScan(t, pool.QueryRow(ctx, `SELECT COALESCE(role, '') FROM stages WHERE bank_id IS NULL AND code = $1`, code), &got)
		if got != want {
			t.Errorf("role of %s = %q, want %q", code, got, want)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE stages SET role = 'sanction' WHERE bank_id IS NULL AND code = 'documentation'`); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatalf("second sanction role err = %v, want 23505", err)
	}
	var n int
	var amount, kind, day string
	mustScan(t, pool.QueryRow(ctx, `SELECT count(*) FROM case_disbursements WHERE case_id = $1`, paid), &n)
	if n != 1 {
		t.Fatalf("payments for old disbursed case = %d, want 1", n)
	}
	mustScan(t, pool.QueryRow(ctx, `SELECT amount::text, kind, paid_on::text FROM case_disbursements WHERE case_id = $1`, paid), &amount, &kind, &day)
	if amount != "500000.00" || kind != "full" || day != "2026-11-05" {
		t.Fatalf("migrated payment = %s %s %s", amount, kind, day)
	}
	mustScan(t, pool.QueryRow(ctx, `SELECT count(*) FROM case_disbursements WHERE case_id = $1`, open), &n)
	if n != 0 {
		t.Fatalf("payments for open case = %d, want 0", n)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO case_disbursements (case_id, amount, paid_on, kind) VALUES ($1, 100, '2026-11-06', 'full')`, paid); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatalf("second full payment err = %v, want 23505", err)
	}
	var partOK bool
	mustScan(t, pool.QueryRow(ctx, `SELECT part_payments FROM banks WHERE id = $1`, bank), &partOK)
	if partOK {
		t.Fatal("part_payments default should be false")
	}
	var ev int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO case_stage_events (case_id, stage_id, reached_at, changed_by, remark)
		VALUES ($1, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), now(), $2, 'moved by mistake') RETURNING id`, open, user), &ev)
	if _, err := pool.Exec(ctx, `UPDATE case_stage_events SET remark = repeat('x', 501) WHERE id = $1`, ev); err == nil || !strings.Contains(err.Error(), "23514") {
		t.Fatalf("501-char remark err = %v, want 23514", err)
	}
}
