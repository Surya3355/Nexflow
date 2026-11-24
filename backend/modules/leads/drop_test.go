package leads

import (
	"context"
	"errors"
	"testing"
)

func TestDropAndBringBackLead(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	got, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, " not now ")
	if err != nil || got.Status != StatusDropped {
		t.Fatalf("drop: %v %s", err, got.Status)
	}
	if n := f.count(t, `SELECT count(*) FROM leads WHERE id = $1 AND dropped_reason_id = $2 AND dropped_remark = 'not now'
		AND dropped_at IS NOT NULL AND dropped_by = $3`, l.ID, f.reason, f.actor.ID); n != 1 {
		t.Error("drop fields not set")
	}
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, ""); !errors.Is(err, ErrLeadClosed) {
		t.Errorf("second drop err = %v", err)
	}
	got, err = f.svc.BringBack(ctx, f.actor, l.ID, testNow)
	if err != nil || got.Status != StatusOpen {
		t.Fatalf("bring back: %v %s", err, got.Status)
	}
	if got.NextCallDate == nil || !got.NextCallDate.Equal(day(2026, 11, 24)) {
		t.Errorf("next call = %v", got.NextCallDate)
	}
	if n := f.count(t, `SELECT count(*) FROM leads WHERE id = $1 AND dropped_reason_id IS NULL AND dropped_remark IS NULL
		AND dropped_at IS NULL AND dropped_by IS NULL`, l.ID); n != 1 {
		t.Error("drop fields not cleared")
	}
	if !f.rec.has("lead.dropped") || !f.rec.has("lead.brought_back") {
		t.Error("missing audit")
	}
	if _, err := f.svc.BringBack(ctx, f.actor, l.ID, testNow); !errors.Is(err, ErrLeadClosed) {
		t.Errorf("bring back open lead err = %v", err)
	}
}

func TestDropLeadNeedsValidReason(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, 0, ""); !errors.Is(err, ErrInvalidReason) {
		t.Errorf("no reason err = %v", err)
	}
	var other int64
	mustScan(t, f.pool.QueryRow(ctx, `SELECT id FROM dropdown_values WHERE category = 'drop_reason' AND value = 'Other'`), &other)
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, other, " "); !errors.Is(err, ErrDropRemarkRequired) {
		t.Errorf("Other without remark err = %v", err)
	}
}

func TestBringBackLeadRevivesDroppedCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE customers SET status='dropped', dropped_reason_id=$2, dropped_at=now() WHERE id=$1`, l.CustomerID, f.reason); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BringBack(ctx, f.actor, l.ID, testNow); err != nil {
		t.Fatalf("bring back: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM customers WHERE id = $1 AND status = 'active'`, l.CustomerID); n != 1 {
		t.Error("customer still dropped")
	}
}

func TestBringBackLeadRefusedWhenOpenDuplicate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddForCustomer(ctx, f.actor, l.CustomerID, f.project, f.actor.ID, false, testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BringBack(ctx, f.actor, l.ID, testNow); !errors.Is(err, ErrLeadExists) {
		t.Errorf("err = %v, want ErrLeadExists", err)
	}
}

// A lead restored as dropped can still be linked to a restored case; it must
// not come back as open (Add case would then clash with that case).
func TestBringBackLeadWithCaseRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, ""); err != nil {
		t.Fatal(err)
	}
	var bank int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, lead_id)
		VALUES ($1, $2, 1, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3, $4)`, l.CustomerID, bank, f.actor.ID, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BringBack(ctx, f.actor, l.ID, testNow); !errors.Is(err, ErrLeadHasCase) {
		t.Fatalf("err = %v, want ErrLeadHasCase", err)
	}
}
