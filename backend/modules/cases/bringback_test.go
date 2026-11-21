package cases

import (
	"context"
	"errors"
	"testing"
)

// dropCustomer marks the fixture customer dropped directly.
func dropCustomer(t *testing.T, f fixture) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE customers SET status = 'dropped', dropped_reason_id = $2, dropped_at = now() WHERE id = $1`,
		f.customer, reasonID(t, f, "Not eligible")); err != nil {
		t.Fatalf("drop customer: %v", err)
	}
}

func TestCreateCaseDroppedCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dropCustomer(t, f)
	in := NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID}

	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrCustomerDropped) {
		t.Fatalf("err = %v, want ErrCustomerDropped", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cases`); n != 0 {
		t.Fatalf("cases = %d, want 0", n)
	}

	in.BringBack = true
	if _, err := f.svc.Create(ctx, f.actor, in); err != nil {
		t.Fatalf("bring back: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM customers WHERE id = $1 AND status = 'active'`, f.customer); n != 1 {
		t.Fatal("customer not active after bring back")
	}
	if ErrCustomerDropped.Error() != "cases: this customer was dropped. Bring them back first" {
		t.Fatalf("message = %q", ErrCustomerDropped.Error())
	}
}

func TestCreateCaseBringBackTwice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dropCustomer(t, f)
	in := NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID, BringBack: true}
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Create(ctx, f.actor, in); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	n := 0
	for _, e := range f.rec.entries {
		if e.Action == "customer.reactivated" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("reactivation audits = %d, want 1", n)
	}
	if c := f.count(t, `SELECT count(*) FROM cases`); c != 2 {
		t.Fatalf("cases = %d, want 2", c)
	}
}
