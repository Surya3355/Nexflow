package leads

import (
	"context"
	"testing"
)

func TestListOpenOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	yesterday := f.newLead(t, "9000000001", f.project) // IST today is 24 Nov
	today := f.newLead(t, "9000000002", f.project)
	tomorrow := f.newLead(t, "9000000003", f.project)
	none := f.newLead(t, "9000000004", f.project)
	converted := f.newLead(t, "9000000005", f.project)
	set := func(id int64, sql string) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, sql, id); err != nil {
			t.Fatal(err)
		}
	}
	set(yesterday.ID, `UPDATE leads SET next_call_date = '2026-11-23', created_at = now() - interval '5 days' WHERE id = $1`)
	set(tomorrow.ID, `UPDATE leads SET next_call_date = '2026-11-25', created_at = now() - interval '2 days' WHERE id = $1`)
	set(none.ID, `UPDATE leads SET next_call_date = NULL, created_at = now() - interval '1 day' WHERE id = $1`)
	set(converted.ID, `UPDATE leads SET status = 'converted', converted_at = now() WHERE id = $1`)

	rows, err := f.svc.ListOpen(ctx, testNow)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []int64{yesterday.ID, today.ID, none.ID, tomorrow.ID}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, id := range want {
		if rows[i].ID != id {
			t.Errorf("row %d = lead %d, want %d", i, rows[i].ID, id)
		}
	}
	if !rows[0].CallDue || rows[0].OverdueDays != 1 {
		t.Errorf("yesterday row due=%v overdue=%d", rows[0].CallDue, rows[0].OverdueDays)
	}
	if !rows[1].CallDue || rows[1].OverdueDays != 0 {
		t.Errorf("today row due=%v overdue=%d", rows[1].CallDue, rows[1].OverdueDays)
	}
	if rows[3].CallDue {
		t.Error("tomorrow row should not be due")
	}
	if rows[0].CustomerName != "Ravi" || rows[0].ProjectName != "Green Villas" || rows[0].AssigneeName != "Staff One" || rows[0].LastResponse != "" {
		t.Errorf("row details = %+v", rows[0])
	}
}

func TestForCustomerListsAllStatuses(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9000000001", f.project)
	if _, err := f.pool.Exec(ctx, `UPDATE leads SET status = 'converted', converted_at = now() WHERE id = $1`, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddForCustomer(ctx, f.actor, l.CustomerID, f.naProject, f.actor.ID, false, testNow); err != nil {
		t.Fatal(err)
	}
	rows, err := f.svc.ForCustomer(ctx, l.CustomerID, testNow)
	if err != nil || len(rows) != 2 {
		t.Fatalf("for customer: %v, %d rows", err, len(rows))
	}
	if rows[0].Status != "open" {
		t.Errorf("newest first: got %s", rows[0].Status)
	}
}
