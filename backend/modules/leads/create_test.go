package leads

import (
	"context"
	"errors"
	"testing"
)

func TestCreateLeadMakesCustomerAndLead(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Create(context.Background(), f.actor,
		NewLead{Name: " Ravi Kumar ", Mobile: "98450 11212", Email: "r@x.in", ProjectID: f.project, AssigneeID: f.other}, testNow)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Customer == nil || res.Lead == nil || res.Existing != nil {
		t.Fatalf("result = %+v", res)
	}
	if res.Customer.Name != "Ravi Kumar" || res.Customer.Mobile != "9845011212" {
		t.Errorf("customer = %+v", res.Customer)
	}
	l := res.Lead
	if l.Status != "open" || l.ProjectID != f.project || l.AssigneeID != f.other || l.CustomerID != res.Customer.ID {
		t.Errorf("lead = %+v", l)
	}
	if l.NextCallDate == nil || !l.NextCallDate.Equal(day(2026, 11, 24)) {
		t.Errorf("next call = %v, want 2026-11-24 (IST day)", l.NextCallDate)
	}
	if !f.rec.has("customer.created") || !f.rec.has("lead.created") {
		t.Errorf("audit = %+v", f.rec.entries)
	}
}

func TestCreateLeadKnownMobileCreatesNothing(t *testing.T) {
	f := newFixture(t)
	f.newLead(t, "9845011212", f.project)
	res, err := f.svc.Create(context.Background(), f.actor,
		NewLead{Name: "Other", Mobile: "9845011212", ProjectID: f.naProject, AssigneeID: f.actor.ID}, testNow)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Existing == nil || res.Lead != nil || res.Customer != nil || res.Existing.Name != "Ravi" {
		t.Fatalf("result = %+v", res)
	}
	if n := f.count(t, `SELECT count(*) FROM customers`); n != 1 {
		t.Errorf("customers = %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM leads`); n != 1 {
		t.Errorf("leads = %d", n)
	}
}

func TestAddLeadBringsBackDroppedCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var cust, bank int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile, status, dropped_reason_id, dropped_at)
		VALUES ('Old', '9000000009', 'dropped', $1, now()) RETURNING id`, f.reason), &cust)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, status, dropped_reason_id, dropped_at)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3, 'dropped', $4, now())`,
		cust, bank, f.actor.ID, f.reason); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddForCustomer(ctx, f.actor, cust, f.project, f.actor.ID, false, testNow); !errors.Is(err, ErrCustomerDropped) {
		t.Fatalf("without bring back err = %v, want ErrCustomerDropped", err)
	}
	l, err := f.svc.AddForCustomer(ctx, f.actor, cust, f.project, f.actor.ID, true, testNow)
	if err != nil || l.Status != "open" {
		t.Fatalf("with bring back: %v %+v", err, l)
	}
	if n := f.count(t, `SELECT count(*) FROM customers WHERE id = $1 AND status = 'active'`, cust); n != 1 {
		t.Error("customer not active")
	}
	if n := f.count(t, `SELECT count(*) FROM cases WHERE customer_id = $1 AND status = 'dropped'`, cust); n != 1 {
		t.Error("old case should stay dropped")
	}
}

func TestAddLeadSameProjectTwiceRefused(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9845011212", f.project)
	ctx := context.Background()
	if _, err := f.svc.AddForCustomer(ctx, f.actor, l.CustomerID, f.project, f.actor.ID, false, testNow); !errors.Is(err, ErrLeadExists) {
		t.Fatalf("same project err = %v, want ErrLeadExists", err)
	}
	if _, err := f.svc.AddForCustomer(ctx, f.actor, l.CustomerID, f.naProject, f.actor.ID, false, testNow); err != nil {
		t.Fatalf("other project: %v", err)
	}
}

func TestCreateLeadNeedsProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := NewLead{Name: "R", Mobile: "9845011212", AssigneeID: f.actor.ID}
	if _, err := f.svc.Create(ctx, f.actor, in, testNow); !errors.Is(err, ErrProjectRequired) {
		t.Errorf("no project err = %v", err)
	}
	in.ProjectID = 99999
	if _, err := f.svc.Create(ctx, f.actor, in, testNow); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v", err)
	}
	in.ProjectID, in.AssigneeID = f.project, 99999
	if _, err := f.svc.Create(ctx, f.actor, in, testNow); !errors.Is(err, ErrAssigneeInvalid) {
		t.Errorf("unknown assignee err = %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM customers`); n != 0 {
		t.Errorf("customers = %d after refused creates", n)
	}
}

func TestAssignLeadQueuesAlert(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9845011212", f.project)
	got, err := f.svc.Assign(context.Background(), f.actor, l.ID, f.other)
	if err != nil || got.AssigneeID != f.other {
		t.Fatalf("assign: %v %+v", err, got)
	}
	if len(f.notifier.calls) != 1 || f.notifier.calls[0] != [2]int64{l.ID, f.other} {
		t.Errorf("notifier = %v", f.notifier.calls)
	}
	if !f.rec.has("lead.assigned") {
		t.Error("no lead.assigned audit")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE leads SET status='dropped', dropped_reason_id=$2, dropped_at=now() WHERE id=$1`, l.ID, f.reason); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Assign(context.Background(), f.actor, l.ID, f.actor.ID); !errors.Is(err, ErrLeadClosed) {
		t.Errorf("dropped lead err = %v, want ErrLeadClosed", err)
	}
}
