package alerts

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeSender records sent mail; fail makes Send return an error.
type fakeSender struct {
	fail   bool
	sent   []string
	bodies []string
}

func (f *fakeSender) Send(_ context.Context, to, subject, body string) error {
	if f.fail {
		return errors.New("smtp down")
	}
	f.sent = append(f.sent, to+"|"+subject)
	f.bodies = append(f.bodies, body)
	return nil
}

type fixture struct {
	pool      *pgxpool.Pool
	cases     *cases.Service
	queue     *Queue
	actor     access.User
	colleague access.User
	customer  int64
	bank      int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	var f fixture
	f.pool = pool
	f.queue = NewQueue(pool)
	f.cases = cases.NewService(pool, workflows.NewService(pool, history.Nop{}), history.NewPgRecorder(pool))
	f.cases.SetNotifier(f.queue)
	f.actor = seedUser(t, pool, "staff", "staff@example.com", access.RoleEmployee)
	f.colleague = seedUser(t, pool, "colleague", "colleague@example.com", access.RoleEmployee)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &f.bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Ravi', '9876543210') RETURNING id`), &f.customer)
	return f
}

func seedUser(t *testing.T, pool *pgxpool.Pool, name, email string, role access.Role) access.User {
	t.Helper()
	var id int64
	mustScan(t, pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role, email) VALUES ($1, $1, 'x', $2, $3) RETURNING id`,
		name, string(role), email), &id)
	return access.User{ID: id, Username: name, Role: role, IsActive: true}
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	mustScan(t, pool.QueryRow(context.Background(), q, args...), &n)
	return n
}

func TestAssignedAlertGoesToNewAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var received int64
	mustScan(t, f.pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`), &received)
	c, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: naProjectID(t, f.pool), CustomerID: f.customer, BankID: f.bank, LoanAmount: "100000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.cases.Assign(ctx, f.actor, c.ID, f.colleague.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}
	var recipient int64
	mustScan(t, f.pool.QueryRow(ctx, `SELECT recipient_user_id FROM alert_log WHERE case_id=$1 AND kind='assigned' ORDER BY id DESC LIMIT 1`, c.ID), &recipient)
	if recipient != f.colleague.ID {
		t.Fatalf("assigned alert recipient = %d, want new assignee %d", recipient, f.colleague.ID)
	}
}

func TestSameAlertNotSentTwice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var caseID int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		SELECT $1, $2, 1000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3 RETURNING id`,
		f.customer, f.bank, f.actor.ID), &caseID)
	for i := 0; i < 2; i++ {
		if err := f.queue.Enqueue(ctx, caseID, f.actor.ID, KindAssigned, nil); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE case_id=$1`, caseID); n != 1 {
		t.Fatalf("alert rows = %d, want 1 (same alert not queued twice)", n)
	}
}

// A failing sender leaves the alert queued; the next run sends it.
func TestAlertFailureIsRetriedNotLost(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var caseID int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		SELECT $1, $2, 1000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3 RETURNING id`,
		f.customer, f.bank, f.colleague.ID), &caseID)
	if err := f.queue.Enqueue(ctx, caseID, f.colleague.ID, KindAssigned, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	down := &fakeSender{fail: true}
	res, err := f.queue.Dispatch(ctx, down, 10)
	if err != nil {
		t.Fatalf("Dispatch with failing sender: %v", err)
	}
	if res.Failed != 1 || res.Sent != 0 {
		t.Fatalf("result = %+v, want 1 failed, 0 sent", res)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE case_id=$1 AND sent_at IS NULL AND attempts = 1`, caseID); n != 1 {
		t.Fatal("failed alert was not kept for retry")
	}

	up := &fakeSender{}
	res, err = f.queue.Dispatch(ctx, up, 10)
	if err != nil {
		t.Fatalf("Dispatch retry: %v", err)
	}
	if res.Sent != 1 || len(up.sent) != 1 {
		t.Fatalf("retry result = %+v sent=%v, want 1 sent", res, up.sent)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE case_id=$1 AND sent_at IS NOT NULL`, caseID); n != 1 {
		t.Fatal("retried alert not marked sent")
	}
}
