package handover

import (
	"context"
	"fmt"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/alerts"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool          *pgxpool.Pool
	admin, a, b   int64
	project, bank int64
	stage         int64
	n             int
}

func scan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	f := &fixture{pool: pool}
	ctx := context.Background()
	for _, u := range []struct {
		name string
		id   *int64
	}{{"admin", &f.admin}, {"alice", &f.a}, {"bob", &f.b}} {
		scan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role, email)
			VALUES ($1, $1, 'x', 'employee', $1 || '@example.com') RETURNING id`, u.name), u.id)
	}
	scan(t, pool.QueryRow(ctx, `SELECT id FROM projects ORDER BY id LIMIT 1`), &f.project)
	scan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('SBI') RETURNING id`), &f.bank)
	scan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code = 'received' AND bank_id IS NULL`), &f.stage)
	return f
}

func (f *fixture) customer(t *testing.T) int64 {
	t.Helper()
	f.n++
	var id int64
	scan(t, f.pool.QueryRow(context.Background(), `INSERT INTO customers (name, mobile) VALUES ('C', $1) RETURNING id`,
		fmt.Sprintf("90000000%02d", f.n)), &id)
	return id
}

func (f *fixture) lead(t *testing.T, assignee int64, status string) int64 {
	t.Helper()
	var id int64
	conv := "NULL"
	if status == "converted" {
		conv = "now()"
	}
	scan(t, f.pool.QueryRow(context.Background(), `INSERT INTO leads (customer_id, project_id, assignee_id, status, converted_at)
		VALUES ($1, $2, $3, $4, `+conv+`) RETURNING id`, f.customer(t), f.project, assignee, status), &id)
	return id
}

func (f *fixture) kase(t *testing.T, assignee int64, status string) int64 {
	t.Helper()
	var id int64
	scan(t, f.pool.QueryRow(context.Background(), `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, status)
		VALUES ($1, $2, 1000, $3, $4, $5) RETURNING id`, f.customer(t), f.bank, f.stage, assignee, status), &id)
	return id
}

func (f *fixture) query(t *testing.T, caseID, assignee int64, number int, status string) int64 {
	t.Helper()
	var id int64
	scan(t, f.pool.QueryRow(context.Background(), `INSERT INTO case_queries (case_id, number, body, raised_on, assignee_id, status, created_by, updated_by)
		VALUES ($1, $2, 'Salary slips', current_date, $3, $4, $3, $3) RETURNING id`, caseID, number, assignee, status), &id)
	return id
}

func assigneeOf(t *testing.T, pool *pgxpool.Pool, table string, id int64) int64 {
	t.Helper()
	var a int64
	scan(t, pool.QueryRow(context.Background(), `SELECT assignee_id FROM `+table+` WHERE id = $1`, id), &a)
	return a
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	scan(t, pool.QueryRow(context.Background(), sql, args...), &n)
	return n
}

func TestReassignAll(t *testing.T) {
	f := newFixture(t)
	openLead1, openLead2 := f.lead(t, f.a, "open"), f.lead(t, f.a, "open")
	convLead := f.lead(t, f.a, "converted")
	active1, active2 := f.kase(t, f.a, "active"), f.kase(t, f.a, "active")
	done := f.kase(t, f.a, "completed")
	q1, q2 := f.query(t, active1, f.a, 1, "open"), f.query(t, active1, f.a, 2, "sent")
	q3 := f.query(t, active2, f.a, 1, "resolved")
	qDone := f.query(t, done, f.a, 1, "open")

	h := New(history.NewPgRecorder(f.pool), alerts.NewQueue(f.pool))
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	leads, cases, queries, err := h.ReassignAllTx(history.WithTx(ctx, tx), f.admin, f.a, f.b)
	if err != nil {
		t.Fatalf("ReassignAllTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if leads != 2 || cases != 2 || queries != 2 {
		t.Fatalf("moved (%d, %d, %d), want (2, 2, 2)", leads, cases, queries)
	}
	for _, c := range []struct {
		table string
		id    int64
		want  int64
	}{
		{"leads", openLead1, f.b}, {"leads", openLead2, f.b}, {"leads", convLead, f.a},
		{"cases", active1, f.b}, {"cases", active2, f.b}, {"cases", done, f.a},
		{"case_queries", q1, f.b}, {"case_queries", q2, f.b}, {"case_queries", q3, f.a}, {"case_queries", qDone, f.a},
	} {
		if got := assigneeOf(t, f.pool, c.table, c.id); got != c.want {
			t.Errorf("%s %d assignee = %d, want %d", c.table, c.id, got, c.want)
		}
	}
	// Alerts: one "assigned" per moved lead and case, none for queries.
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE recipient_user_id = $1 AND kind = 'assigned'`, f.b); n != 4 {
		t.Errorf("assigned alerts = %d, want 4", n)
	}
	// History: each move recorded, with the handover reason, by the admin.
	for action, want := range map[string]int{"lead.assigned": 2, "case.assigned": 2, "query.reassigned": 2} {
		n := count(t, f.pool, `SELECT count(*) FROM audit_log WHERE action = $1 AND actor_user_id = $2 AND details->>'reason' = 'handover'`, action, f.admin)
		if n != want {
			t.Errorf("%s entries = %d, want %d", action, n, want)
		}
	}
	// Case timeline gets an assignment event per moved case.
	if n := count(t, f.pool, `SELECT count(*) FROM case_assignment_events WHERE from_user_id = $1 AND to_user_id = $2`, f.a, f.b); n != 2 {
		t.Errorf("case assignment events = %d, want 2", n)
	}
}

func TestReassignAllNeedsTx(t *testing.T) {
	f := newFixture(t)
	h := New(history.NewPgRecorder(f.pool), alerts.NewQueue(f.pool))
	if _, _, _, err := h.ReassignAllTx(context.Background(), f.admin, f.a, f.b); err == nil {
		t.Fatal("ran without a transaction")
	}
}
