package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads"
)

func assignedAlerts(t *testing.T, f fixture, recipient int64) int {
	t.Helper()
	return count(t, f.pool, `SELECT count(*) FROM alert_log WHERE recipient_user_id = $1 AND kind = 'assigned'`, recipient)
}

func naProject(t *testing.T, f fixture) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT id FROM projects WHERE is_system`), &id)
	return id
}

func TestCreateCaseAssignedToOtherQueuesAlert(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	na := naProject(t, f)
	if _, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: na, CustomerID: f.customer, BankID: f.bank, LoanAmount: "100000", AssigneeID: f.colleague.ID}); err != nil {
		t.Fatal(err)
	}
	if n := assignedAlerts(t, f, f.colleague.ID); n != 1 {
		t.Fatalf("alerts for the colleague = %d, want 1", n)
	}
	if _, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: na, CustomerID: f.customer, BankID: f.bank, LoanAmount: "100000", AssigneeID: f.actor.ID}); err != nil {
		t.Fatal(err)
	}
	if n := assignedAlerts(t, f, f.actor.ID); n != 0 {
		t.Fatalf("alerts for self-assigned = %d, want 0", n)
	}
}

func TestCreateLeadAssignedToOtherQueuesAlert(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	svc := leads.NewService(f.pool, history.Nop{})
	svc.SetNotifier(f.queue)
	na := naProject(t, f)
	res, err := svc.Create(ctx, f.actor, leads.NewLead{Name: "Asha", Mobile: "9123400001", ProjectID: na, AssigneeID: f.colleague.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n := assignedAlerts(t, f, f.colleague.ID); n != 1 {
		t.Fatalf("lead alerts for the colleague = %d, want 1", n)
	}
	// Turning the lead into a case for the colleague: the case alert too.
	if _, err := f.cases.CreateFromLead(ctx, f.actor, res.Lead.ID, cases.FromLead{BankID: f.bank, LoanAmount: "100000", AssigneeID: f.colleague.ID, StartAt: "received"}); err != nil {
		t.Fatal(err)
	}
	if n := assignedAlerts(t, f, f.colleague.ID); n != 2 {
		t.Fatalf("alerts for the colleague after Add case = %d, want 2", n)
	}
	if _, err := svc.Create(ctx, f.actor, leads.NewLead{Name: "Self", Mobile: "9123400002", ProjectID: na, AssigneeID: f.actor.ID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := assignedAlerts(t, f, f.actor.ID); n != 0 {
		t.Fatalf("alerts for self-assigned lead = %d, want 0", n)
	}
}
