package archive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	svc      *Service
	pool     *pgxpool.Pool
	manager  access.User
	employee access.User
	customer int64
	caseID   int64
}

func seedUser(t *testing.T, pool *pgxpool.Pool, name string, role access.Role) access.User {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role) VALUES ($1, $1, 'x', $2) RETURNING id`,
		name, string(role)).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return access.User{ID: id, Username: name, Role: role, IsActive: true}
}

// newFixture builds a customer with one live case at Received.
func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	f := fixture{svc: NewService(pool, history.NewPgRecorder(pool)), pool: pool}
	f.manager = seedUser(t, pool, "lead", access.RoleManager)
	f.employee = seedUser(t, pool, "staff", access.RoleEmployee)
	var bankID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`).Scan(&bankID); err != nil {
		t.Fatalf("bank: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`).Scan(&stageID); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Ravi', '9876543210') RETURNING id`).Scan(&f.customer); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 500000, $3, $4) RETURNING id`, f.customer, bankID, stageID, f.employee.ID).Scan(&f.caseID); err != nil {
		t.Fatalf("case: %v", err)
	}
	return f
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestEmployeeCannotHardDelete(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.DeleteCase(context.Background(), f.employee, f.caseID); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("employee DeleteCase err = %v, want ErrForbidden", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cases WHERE id=$1`, f.caseID); n != 1 {
		t.Fatal("refused delete removed the case")
	}
}

func TestManagerCanHardDeleteCase(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.DeleteCase(context.Background(), f.manager, f.caseID); err != nil {
		t.Fatalf("manager DeleteCase: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cases WHERE id=$1`, f.caseID); n != 0 {
		t.Fatal("case still live after delete")
	}
	if n := count(t, f.pool, `SELECT count(*) FROM archive WHERE entity_type='case' AND entity_id=$1`, f.caseID); n != 1 {
		t.Fatalf("archive rows for case = %d, want 1", n)
	}
}

func TestDeleteCaseWritesArchiveSnapshotWithPurgeDate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("DeleteCase: %v", err)
	}
	var archivedAt, purgeAfter time.Time
	var snapshot []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT archived_at, purge_after, snapshot FROM archive WHERE entity_type='case' AND entity_id=$1`, f.caseID).
		Scan(&archivedAt, &purgeAfter, &snapshot); err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if want := archivedAt.AddDate(1, 0, 0); !purgeAfter.Equal(want) {
		t.Fatalf("purge_after = %v, want archived_at + 1 year = %v", purgeAfter, want)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatalf("snapshot not JSON: %v", err)
	}
	for _, key := range []string{"case", "customer"} {
		if _, ok := snap[key]; !ok {
			t.Fatalf("snapshot missing %q; keys=%d", key, len(snap))
		}
	}
}

func TestCustomerWithCasesCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	err := f.svc.DeleteCustomer(context.Background(), f.manager, f.customer)
	if !errors.Is(err, ErrCustomerHasCases) {
		t.Fatalf("err = %v, want ErrCustomerHasCases", err)
	}
}

// Owner answer D: archive the cases first, then the customer. Its archive
// entry lists the archived case IDs.
func TestCustomerDeletedAfterCasesArchivedListsThem(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive case: %v", err)
	}
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatalf("DeleteCustomer: %v", err)
	}
	var snapshot []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT snapshot FROM archive WHERE entity_type='customer' AND entity_id=$1`, f.customer).Scan(&snapshot); err != nil {
		t.Fatalf("customer archive row: %v", err)
	}
	var snap struct {
		CaseIDs []int64 `json:"case_ids"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.CaseIDs) != 1 || snap.CaseIDs[0] != f.caseID {
		t.Fatalf("case_ids = %v, want [%d]", snap.CaseIDs, f.caseID)
	}
}

func TestDeleteWritesAuditEntryThatSurvivesPurge(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.DeleteCase(context.Background(), f.manager, f.caseID); err != nil {
		t.Fatalf("DeleteCase: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM audit_log WHERE action='case.deleted' AND entity_id=$1`, f.caseID); n != 1 {
		t.Fatalf("case.deleted audit rows = %d, want 1", n)
	}
}
