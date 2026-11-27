package leads

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testNow is 20:00 UTC on 23 Nov, which is already 24 Nov in IST.
var testNow = time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)

type recorder struct{ entries []history.Entry }

func (r *recorder) Record(_ context.Context, e history.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func (r *recorder) has(action string) bool {
	for _, e := range r.entries {
		if e.Action == action {
			return true
		}
	}
	return false
}

type fakeNotifier struct{ calls [][2]int64 }

func (n *fakeNotifier) QueueLeadAssigned(_ context.Context, leadID, recipientID int64) error {
	n.calls = append(n.calls, [2]int64{leadID, recipientID})
	return nil
}

type fixture struct {
	svc       *Service
	pool      *pgxpool.Pool
	actor     access.User
	other     int64
	naProject int64
	project   int64
	reason    int64
	rec       *recorder
	notifier  *fakeNotifier
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	f := fixture{pool: pool, rec: &recorder{}, notifier: &fakeNotifier{}}
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('staff', 'Staff One', 'x', 'employee') RETURNING id`), &f.actor.ID)
	f.actor.Username, f.actor.Role, f.actor.IsActive = "staff", access.RoleEmployee, true
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('anil', 'Anil', 'x', 'employee') RETURNING id`), &f.other)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM projects WHERE is_system`), &f.naProject)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO projects (name) VALUES ('Green Villas') RETURNING id`), &f.project)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM dropdown_values WHERE category = 'drop_reason' ORDER BY sort_order LIMIT 1`), &f.reason)
	f.svc = NewService(pool, f.rec)
	f.svc.SetNotifier(f.notifier)
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

// newLead creates a lead for a fresh customer with the given mobile.
func (f fixture) newLead(t *testing.T, mobile string, project int64) Lead {
	t.Helper()
	res, err := f.svc.Create(context.Background(), f.actor, NewLead{Name: "Ravi", Mobile: mobile, ProjectID: project, AssigneeID: f.actor.ID}, testNow)
	if err != nil || res.Lead == nil {
		t.Fatalf("create lead: %v (%+v)", err, res)
	}
	return *res.Lead
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
