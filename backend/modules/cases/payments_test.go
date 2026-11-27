package cases

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func manager(t *testing.T, f fixture) access.User {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('boss', 'Boss', 'x', 'manager') RETURNING id`), &id)
	return access.User{ID: id, Username: "boss", Role: access.RoleManager, IsActive: true}
}

// hdfcAtDisbursement opens an HDFC case sanctioned at sanctioned and moves it to
// Disbursement with a first part payment of first.
func hdfcAtDisbursement(t *testing.T, f fixture, sanctioned, first string) (int64, Case) {
	t.Helper()
	ctx := context.Background()
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	if sanctioned != "" {
		if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "sanctioned"),
			Sanction: &SanctionDetails{Amount: sanctioned, RatePercent: "8.5"}}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "disbursement"), Payment: part(first)})
	if err != nil {
		t.Fatal(err)
	}
	return bank, got
}

func TestAddPaymentAtDisbursementOnly(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.svc.AddPayment(context.Background(), f.actor, c.ID, *full("1000")); !errors.Is(err, ErrNotAtDisbursement) {
		t.Fatalf("err = %v, want ErrNotAtDisbursement", err)
	}
}

func TestSummaryRemainingUsesSanctioned(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *part("1500000")); err != nil {
		t.Fatal(err)
	}
	sum, err := f.svc.Payments(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Payments) != 2 || sum.TotalPaid != "2500000.00" || sum.Remaining != "1000000.00" || sum.FullyPaid || sum.OverPaid {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestRemainingFallsBackToLoanAmount(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "", "1000000")
	sum, err := f.svc.Payments(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Remaining != "4000000.00" {
		t.Fatalf("remaining = %s, want 4000000.00 (loan 5000000 - 1000000)", sum.Remaining)
	}
}

func TestOverPaidFlagNotBlocked(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "2000000", "1500000")
	ctx := context.Background()
	got, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("1000000"))
	if err != nil {
		t.Fatalf("over-payment should save: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	sum, _ := f.svc.Payments(ctx, c.ID)
	if sum.Remaining != "0.00" || !sum.OverPaid || !sum.FullyPaid {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestConcurrentFullOnlyOnce(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.svc.AddPayment(ctx, f.actor, c.ID, *full("2500000"))
		}(i)
	}
	wg.Wait()
	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyFullyPaid):
			dup++
		default:
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if ok != 1 || dup != 1 {
		t.Fatalf("ok = %d, dup = %d", ok, dup)
	}
	if n := f.count(t, `SELECT count(*) FROM case_disbursements WHERE case_id = $1 AND kind = 'full'`, c.ID); n != 1 {
		t.Fatalf("full rows = %d", n)
	}
}

func TestEmployeeCannotEditOrDelete(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	sum, _ := f.svc.Payments(ctx, c.ID)
	id := sum.Payments[0].ID
	if _, err := f.svc.UpdatePayment(ctx, f.actor, c.ID, id, *part("900000")); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("update err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.DeletePayment(ctx, f.actor, c.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("delete err = %v, want ErrForbidden", err)
	}
	boss := manager(t, f)
	if _, err := f.svc.UpdatePayment(ctx, boss, c.ID, id, *part("900000")); err != nil {
		t.Fatalf("manager update: %v", err)
	}
	sum, _ = f.svc.Payments(ctx, c.ID)
	if sum.TotalPaid != "900000.00" {
		t.Fatalf("total after edit = %s", sum.TotalPaid)
	}
	other := newCase(t, f)
	if _, err := f.svc.DeletePayment(ctx, boss, other.ID, id); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("payment of another case err = %v, want ErrPaymentNotFound", err)
	}
}

func TestDeleteFullReopensCase(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("2500000")); err != nil {
		t.Fatal(err)
	}
	sum, _ := f.svc.Payments(ctx, c.ID)
	var fullID int64
	for _, p := range sum.Payments {
		if p.Kind == PaymentFull {
			fullID = p.ID
		}
	}
	got, err := f.svc.DeletePayment(ctx, manager(t, f), c.ID, fullID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %s, want active", got.Status)
	}
}

func TestChangeFullToPartReopensCase(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("2500000")); err != nil {
		t.Fatal(err)
	}
	sum, _ := f.svc.Payments(ctx, c.ID)
	got, err := f.svc.UpdatePayment(ctx, manager(t, f), c.ID, sum.Payments[1].ID, *part("2500000"))
	if err != nil || got.Status != "active" {
		t.Fatalf("status = %s, %v; want active", got.Status, err)
	}
}

func TestDeleteAllPaymentsLeavesActive(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	sum, _ := f.svc.Payments(ctx, c.ID)
	got, err := f.svc.DeletePayment(ctx, manager(t, f), c.ID, sum.Payments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	sum, _ = f.svc.Payments(ctx, c.ID)
	if got.Status != "active" || len(sum.Payments) != 0 || sum.Remaining != "3500000.00" {
		t.Fatalf("status %s, summary %+v", got.Status, sum)
	}
}

func TestPaymentChangesAudited(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *part("500000")); err != nil {
		t.Fatal(err)
	}
	sum, _ := f.svc.Payments(ctx, c.ID)
	boss := manager(t, f)
	if _, err := f.svc.UpdatePayment(ctx, boss, c.ID, sum.Payments[1].ID, *part("600000")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.DeletePayment(ctx, boss, c.ID, sum.Payments[1].ID); err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]any{}
	for _, e := range f.rec.entries {
		d, _ := e.Details.(map[string]any)
		seen[e.Action] = d
	}
	for _, a := range []string{"case.payment_added", "case.payment_updated", "case.payment_deleted"} {
		if _, ok := seen[a]; !ok {
			t.Errorf("missing audit %s", a)
		}
	}
	if b, _ := seen["case.payment_updated"]["before"].(map[string]any); b == nil || b["amount"] != "500000.00" {
		t.Errorf("update audit before = %v", seen["case.payment_updated"]["before"])
	}
}

func TestCasePartPaymentsFlag(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	got, err := f.svc.Get(context.Background(), c.ID)
	if err != nil || !got.PartPayments {
		t.Fatalf("part payments = %v, %v", got.PartPayments, err)
	}
	std := newCase(t, f)
	got, _ = f.svc.Get(context.Background(), std.ID)
	if got.PartPayments {
		t.Fatal("standard bank case should not offer part payments")
	}
}
