package bankqueries

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	svc    *Service
	docs   *documents.Service
	store  *trackingStore
	pool   *pgxpool.Pool
	staff  access.User
	other  access.User
	caseID int64
}

// trackingStore remembers every key written, to find leftover files.
type trackingStore struct {
	*documents.MemoryStore
	keys []string
}

func (s *trackingStore) Put(key string, data []byte) error {
	s.keys = append(s.keys, key)
	return s.MemoryStore.Put(key, data)
}

// live counts the written files still in the store.
func (s *trackingStore) live() int {
	n := 0
	for _, k := range s.keys {
		if s.Has(k) {
			n++
		}
	}
	return n
}

func newUser(t *testing.T, pool *pgxpool.Pool, name string) access.User {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ($1, $1, 'x', 'employee') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return access.User{ID: id, Username: name, Role: access.RoleEmployee, IsActive: true}
}

func setup(t *testing.T, rec history.Recorder) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		rec = history.NewPgRecorder(pool)
	}
	f := fixture{pool: pool, store: &trackingStore{MemoryStore: documents.NewMemoryStore()}}
	f.docs = documents.NewService(pool, f.store, rec)
	f.svc = NewService(pool, f.docs, rec)
	f.staff = newUser(t, pool, "ravi")
	f.other = newUser(t, pool, "anita")
	f.caseID = newCase(t, pool, f.staff.ID)
	return f
}

func newCase(t *testing.T, pool *pgxpool.Pool, assignee int64) int64 {
	t.Helper()
	ctx := context.Background()
	var bank, cust, id int64
	if err := pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('B' || (SELECT count(*) FROM banks)) RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('C', '9' || lpad((SELECT count(*) FROM customers)::text, 9, '0')) RETURNING id`).Scan(&cust); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3) RETURNING id`, cust, bank, assignee).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func dropCase(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE cases SET status = 'dropped', dropped_at = now(),
		dropped_reason_id = (SELECT id FROM dropdown_values WHERE category = 'drop_reason' ORDER BY id LIMIT 1) WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func today() time.Time {
	n := time.Now().In(time.FixedZone("IST", 5*60*60+30*60))
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func (f fixture) add(t *testing.T, body string) Query {
	t.Helper()
	q, err := f.svc.Add(context.Background(), f.staff, f.caseID, NewQuery{Body: body, RaisedOn: today(), AssigneeID: f.staff.ID})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return q
}

func TestAddNumbersQueriesAndRecordsRaised(t *testing.T) {
	f := setup(t, nil)
	due := today().AddDate(0, 0, 2)
	q1, err := f.svc.Add(context.Background(), f.staff, f.caseID, NewQuery{Body: "  Salary slips  ", RaisedOn: today(), DueOn: &due, AssigneeID: f.other.ID})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if q1.Number != 1 || q1.Body != "Salary slips" || q1.Status != StatusOpen || q1.AssigneeName != "anita" || q1.DueOn == nil || !q1.DueOn.Equal(due) {
		t.Fatalf("q1 = %+v", q1)
	}
	if len(q1.Events) != 1 || q1.Events[0].Kind != "raised" || q1.Events[0].By != "ravi" {
		t.Fatalf("events = %+v, want one raised by ravi", q1.Events)
	}
	q2 := f.add(t, "NOC")
	if q2.Number != 2 {
		t.Fatalf("second number = %d, want 2", q2.Number)
	}
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'query.added'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit query.added = %d (%v), want 2", n, err)
	}
}

func TestAddValidation(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	before := today().AddDate(0, 0, -1)
	cases := map[string]struct {
		in   NewQuery
		want error
	}{
		"blank":        {NewQuery{Body: "  ", RaisedOn: today(), AssigneeID: f.staff.ID}, ErrBodyRequired},
		"long":         {NewQuery{Body: strings.Repeat("x", 501), RaisedOn: today(), AssigneeID: f.staff.ID}, ErrBodyTooLong},
		"future":       {NewQuery{Body: "x", RaisedOn: today().AddDate(0, 0, 1), AssigneeID: f.staff.ID}, ErrRaisedInFuture},
		"due < raised": {NewQuery{Body: "x", RaisedOn: today(), DueOn: &before, AssigneeID: f.staff.ID}, ErrDueBeforeRaised},
		"no assignee":  {NewQuery{Body: "x", RaisedOn: today(), AssigneeID: 99999}, ErrAssigneeInactive},
	}
	for name, c := range cases {
		if _, err := f.svc.Add(ctx, f.staff, f.caseID, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, f.other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Add(ctx, f.staff, f.caseID, NewQuery{Body: "x", RaisedOn: today(), AssigneeID: f.other.ID}); !errors.Is(err, ErrAssigneeInactive) {
		t.Errorf("inactive assignee err = %v", err)
	}
	if _, err := f.svc.Add(ctx, f.staff, 99999, NewQuery{Body: "x", RaisedOn: today(), AssigneeID: f.staff.ID}); !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("missing case err = %v", err)
	}
	dropCase(t, f.pool, f.caseID)
	if _, err := f.svc.Add(ctx, f.staff, f.caseID, NewQuery{Body: "x", RaisedOn: today(), AssigneeID: f.staff.ID}); !errors.Is(err, ErrCaseDropped) {
		t.Errorf("dropped case err = %v", err)
	}
}

func TestEditChangesBodyDueAndAssignee(t *testing.T) {
	f := setup(t, nil)
	q := f.add(t, "Salary slips")
	due := today().AddDate(0, 0, 3)
	got, err := f.svc.Edit(context.Background(), f.staff, q.ID, NewQuery{Body: "Salary slips (3 months)", RaisedOn: today(), DueOn: &due, AssigneeID: f.other.ID})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got.Body != "Salary slips (3 months)" || got.AssigneeID != f.other.ID || got.DueOn == nil || got.Events[len(got.Events)-1].Kind != "edited" {
		t.Fatalf("edited = %+v", got)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE case_queries SET status = 'resolved' WHERE id = $1`, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Edit(context.Background(), f.staff, q.ID, NewQuery{Body: "x", RaisedOn: today(), AssigneeID: f.staff.ID}); !errors.Is(err, ErrQueryResolved) {
		t.Fatalf("edit resolved err = %v, want ErrQueryResolved", err)
	}
	if _, err := f.svc.Edit(context.Background(), f.staff, 99999, NewQuery{Body: "x", RaisedOn: today(), AssigneeID: f.staff.ID}); !errors.Is(err, ErrQueryNotFound) {
		t.Fatalf("edit missing err = %v", err)
	}
}

func TestListOrderAndCount(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	a := f.add(t, "A")
	b := f.add(t, "B")
	c := f.add(t, "C")
	if _, err := f.pool.Exec(ctx, `UPDATE case_queries SET status = 'resolved' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE case_queries SET removed_at = now(), removed_by = $2 WHERE id = $1`, b.ID, f.staff.ID); err != nil {
		t.Fatal(err)
	}
	other := newCase(t, f.pool, f.staff.ID)
	if _, err := f.svc.Add(ctx, f.staff, other, NewQuery{Body: "elsewhere", RaisedOn: today(), AssigneeID: f.staff.ID}); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.List(ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != c.ID || list[1].ID != a.ID {
		t.Fatalf("list = %+v, want C (unresolved) then A (resolved), B hidden", list)
	}
	n, err := f.svc.OpenCount(ctx, f.caseID)
	if err != nil || n != 1 {
		t.Fatalf("OpenCount = %d (%v), want 1", n, err)
	}
}
