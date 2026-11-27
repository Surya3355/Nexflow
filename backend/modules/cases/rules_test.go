package cases

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAmountRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	for _, bad := range []string{"100.555", "999999999999999999", "0", "1e5", "1000000000.01"} {
		_, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: bad, AssigneeID: f.actor.ID})
		if !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("create with %q: err = %v, want ErrInvalidAmount", bad, err)
		}
		if _, err := f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: bad}); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("edit with %q: err = %v, want ErrInvalidAmount", bad, err)
		}
	}
	if _, err := f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: "1000000000"}); err != nil {
		t.Errorf("edit with ₹100 crore: %v", err)
	}
}

func TestSanctionAndPaymentAmountRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	_, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "sanctioned"),
		Sanction: &SanctionDetails{Amount: "100.555", RatePercent: "8.5"}})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("sanction 100.555: err = %v, want ErrInvalidAmount", err)
	}
}

func TestPaymentAmountRule(t *testing.T) {
	f := newFixture(t)
	_, d := hdfcAtDisbursement(t, f, "3500000", "1000000")
	if _, err := f.svc.AddPayment(context.Background(), f.actor, d.ID, *part("2000000000")); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("payment ₹200 crore: err = %v, want ErrInvalidAmount", err)
	}
}

func TestPaymentFutureDate(t *testing.T) {
	f := newFixture(t)
	// 2026-11-27 18:00 UTC is 23:30 IST on the 27th.
	f.svc.SetClock(func() time.Time { return time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC) })
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	tomorrow := PaymentInput{Amount: "1000", Kind: PaymentPart, Date: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)}
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, tomorrow); !errors.Is(err, ErrPaymentDateFuture) {
		t.Fatalf("tomorrow IST: err = %v, want ErrPaymentDateFuture", err)
	}
	today := PaymentInput{Amount: "1000", Kind: PaymentPart, Date: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)}
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, today); err != nil {
		t.Fatalf("today IST at 23:30: %v", err)
	}
}

func TestPaymentDateTodayIST(t *testing.T) {
	f := newFixture(t)
	// 2026-11-27 20:00 UTC is already 28 Nov 01:30 IST: the 28th is today.
	f.svc.SetClock(func() time.Time { return time.Date(2026, 11, 27, 20, 0, 0, 0, time.UTC) })
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	in := PaymentInput{Amount: "1000", Kind: PaymentPart, Date: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)}
	if _, err := f.svc.AddPayment(context.Background(), f.actor, c.ID, in); err != nil {
		t.Fatalf("28 Nov at 01:30 IST: %v", err)
	}
}

func TestPaymentUpdateFutureDate(t *testing.T) {
	f := newFixture(t)
	f.svc.SetClock(func() time.Time { return time.Date(2026, 11, 27, 6, 0, 0, 0, time.UTC) })
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	ctx := context.Background()
	sum, err := f.svc.Payments(ctx, c.ID)
	if err != nil || len(sum.Payments) == 0 {
		t.Fatalf("payments: %v", err)
	}
	in := PaymentInput{Amount: "1000", Kind: PaymentPart, Date: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := f.svc.UpdatePayment(ctx, manager(t, f), c.ID, sum.Payments[0].ID, in); !errors.Is(err, ErrPaymentDateFuture) {
		t.Fatalf("edit to 2027: err = %v, want ErrPaymentDateFuture", err)
	}
}

func TestBadInterestRateNot500(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	for _, rate := range []string{"abc", "-1", "101"} {
		_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "sanctioned"),
			Sanction: &SanctionDetails{Amount: "3000000", RatePercent: rate}})
		if !errors.Is(err, ErrInvalidRate) {
			t.Errorf("rate %q: err = %v, want ErrInvalidRate", rate, err)
		}
	}
}

func TestMoveToCurrentStage(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	before := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID)
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: c.CurrentStageID})
	if !errors.Is(err, ErrAlreadyAtStage) {
		t.Fatalf("move to current stage: err = %v, want ErrAlreadyAtStage", err)
	}
	if after := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); after != before {
		t.Fatalf("history rows %d -> %d, want no new row", before, after)
	}
}

func TestChangeStageReturnsFullCase(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	project := addProject(t, f, "Prestige")
	if _, err := f.pool.Exec(ctx, `UPDATE projects SET type = 'villas', address = 'Road 1' WHERE id = $1`, project); err != nil {
		t.Fatal(err)
	}
	in := caseOn(f, project, addContact(t, f, project, "9876500001"))
	in.UnitNumber = "B-1402"
	c, err := f.svc.Create(ctx, f.actor, in)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("AB123")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectType == nil || got.BuilderContact == nil {
		t.Fatal("fixture: Get has no project type or builder contact")
	}
	if moved.ProjectType == nil || moved.BuilderContact == nil {
		t.Fatalf("ChangeStage case differs from Get: project type %v vs %v, contact %v vs %v",
			moved.ProjectType, got.ProjectType, moved.BuilderContact, got.BuilderContact)
	}
}
