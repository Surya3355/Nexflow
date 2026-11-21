package cases

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fixture is a migrated database with a bank, a customer, and an employee.
type fixture struct {
	svc      *Service
	pool     *pgxpool.Pool
	actor    access.User
	bankID   int64
	customer int64
	rec      *recorder
	naProject int64
}

type recorder struct{ entries []history.Entry }

func (r *recorder) Record(_ context.Context, e history.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	var f fixture
	f.rec = &recorder{}
	f.pool = pool
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('staff', 'Staff', 'x', 'employee') RETURNING id`), &f.actor.ID)
	f.actor.Username, f.actor.Role, f.actor.IsActive = "staff", access.RoleEmployee, true
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &f.bankID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Ravi', '9876543210') RETURNING id`), &f.customer)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM projects WHERE is_system`), &f.naProject)
	f.svc = NewService(pool, workflows.NewService(pool, history.Nop{}), f.rec)
	return f
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func (f fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	mustScan(t, f.pool.QueryRow(context.Background(), query, args...), &n)
	return n
}

func TestNewCaseStartsAtReceived(t *testing.T) {
	f := newFixture(t)
	c, err := f.svc.Create(context.Background(), f.actor, NewCase{
		ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "2500000", AssigneeID: f.actor.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var code string
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT code FROM stages WHERE id = $1`, c.CurrentStageID), &code)
	if code != "received" {
		t.Fatalf("new case stage = %s, want received", code)
	}
}

func TestNewCaseWritesStageAndAssignmentEvents(t *testing.T) {
	f := newFixture(t)
	c, err := f.svc.Create(context.Background(), f.actor, NewCase{
		ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); n != 1 {
		t.Fatalf("stage events = %d, want 1", n)
	}
	if n := f.count(t, `SELECT count(*) FROM case_assignment_events WHERE case_id = $1 AND from_user_id IS NULL`, c.ID); n != 1 {
		t.Fatalf("assignment events with null from_user = %d, want 1", n)
	}
}

func TestLoanAmountMustBePositive(t *testing.T) {
	f := newFixture(t)
	for _, amount := range []string{"0", "-5", "abc", ""} {
		_, err := f.svc.Create(context.Background(), f.actor, NewCase{
			ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: amount, AssigneeID: f.actor.ID,
		})
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("amount %q err = %v, want ErrInvalidAmount", amount, err)
		}
	}
}

func TestCustomerCanHaveSeveralCases(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Create(context.Background(), f.actor, NewCase{
			ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "500000", AssigneeID: f.actor.ID,
		}); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM cases WHERE customer_id = $1`, f.customer); n != 2 {
		t.Fatalf("cases for customer = %d, want 2", n)
	}
}

func TestNewCaseWritesAuditEntry(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Create(context.Background(), f.actor, NewCase{
		ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "1000", AssigneeID: f.actor.ID,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, e := range f.rec.entries {
		if e.Action == "case.created" {
			return
		}
	}
	t.Fatalf("no case.created audit entry; got %+v", f.rec.entries)
}
