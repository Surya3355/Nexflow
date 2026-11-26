package cases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

// hdfcBank adds a bank on the HDFC stage list (part payments on).
func hdfcBank(t *testing.T, f fixture) int64 {
	t.Helper()
	ctx := context.Background()
	admin := access.User{ID: f.actor.ID, Role: access.RoleAdmin, IsActive: true}
	wf := workflows.NewService(f.pool, history.Nop{})
	b, err := wf.CreateBank(ctx, admin, "HDFC Ltd")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.SetBankStageList(ctx, admin, b.ID, workflows.StageListHDFC); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func bankStage(t *testing.T, f fixture, bank int64, code string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT id FROM stages WHERE bank_id = $1 AND code = $2`, bank, code), &id)
	return id
}

func caseAtBank(t *testing.T, f fixture, bank int64, amount string) Case {
	t.Helper()
	c, err := f.svc.Create(context.Background(), f.actor, NewCase{
		ProjectID: f.naProject, CustomerID: f.customer, BankID: bank, LoanAmount: amount, AssigneeID: f.actor.ID,
	})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	return c
}

func full(amount string) *PaymentInput {
	return &PaymentInput{Amount: amount, Date: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Kind: PaymentFull}
}

func part(amount string) *PaymentInput {
	return &PaymentInput{Amount: amount, Date: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), Kind: PaymentPart}
}

func TestTrackingRoleOnHDFCApplicationSubmitted(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	ctx := context.Background()
	_, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "application_submitted")})
	if !errors.Is(err, ErrTrackingRequired) {
		t.Fatalf("err = %v, want ErrTrackingRequired", err)
	}
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "application_submitted"), TrackingNumber: tracking("hd123")})
	if err != nil || got.TrackingNumber == nil || *got.TrackingNumber != "HD123" {
		t.Fatalf("move with tracking = %+v, %v", got.TrackingNumber, err)
	}
	// Logged-in rule no longer applies by code: DOVR needs nothing.
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "dovr")}); err != nil {
		t.Fatalf("dovr: %v", err)
	}
}

func TestSanctionOverwritesLoanAmount(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	got, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{
		ToStageID: bankStage(t, f, bank, "sanctioned"),
		Sanction:  &SanctionDetails{Amount: "4500000", RatePercent: "8.5", InterestType: "floating", TenureMonths: 240},
	})
	if err != nil {
		t.Fatalf("sanction: %v", err)
	}
	if got.LoanAmount != "4500000.00" {
		t.Fatalf("loan amount = %s, want 4500000.00", got.LoanAmount)
	}
	var sanctioned string
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT sanctioned_amount::text FROM cases WHERE id = $1`, c.ID), &sanctioned)
	if sanctioned != "4500000.00" {
		t.Fatalf("sanctioned = %s", sanctioned)
	}
	var found bool
	for _, e := range f.rec.entries {
		d, _ := e.Details.(map[string]any)
		if e.Action == "case.stage_changed" && d["old_loan_amount"] == "5000000.00" {
			found = true
		}
	}
	if !found {
		t.Fatal("audit should keep the old loan amount")
	}
}

func TestSanctionNeedsAmount(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	ctx := context.Background()
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "sanctioned")}); !errors.Is(err, ErrSanctionAmountRequired) {
		t.Fatalf("no sanction err = %v, want ErrSanctionAmountRequired", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "sanctioned"), Sanction: &SanctionDetails{Amount: " "}}); !errors.Is(err, ErrSanctionAmountRequired) {
		t.Fatalf("blank amount err = %v, want ErrSanctionAmountRequired", err)
	}
}

func TestRemarkSavedAndOptional(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	ctx := context.Background()
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("RMK1"), Remark: "  sent back by bank  "}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "received")}); err != nil {
		t.Fatalf("move back without remark: %v", err)
	}
	var withRemark, without int
	mustScan(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM case_stage_events WHERE case_id = $1 AND remark = 'sent back by bank'`, c.ID), &withRemark)
	mustScan(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM case_stage_events WHERE case_id = $1 AND remark IS NULL`, c.ID), &without)
	if withRemark != 1 || without != 2 {
		t.Fatalf("remark rows = %d, null rows = %d", withRemark, without)
	}
}

func TestRemarkTooLong(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "under_sanction"), Remark: strings.Repeat("x", 501)})
	if !errors.Is(err, ErrRemarkTooLong) {
		t.Fatalf("err = %v, want ErrRemarkTooLong", err)
	}
}

func TestDisbursementNeedsFirstPayment(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed")}); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("err = %v, want ErrPaymentRequired", err)
	}
}

func TestPartPaymentKeepsCaseActive(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "3500000")
	got, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "disbursement"), Payment: part("1000000")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %s, want active", got.Status)
	}
	if n := f.count(t, `SELECT count(*) FROM case_disbursements WHERE case_id = $1 AND kind = 'part'`, c.ID); n != 1 {
		t.Fatalf("part rows = %d", n)
	}
}

func TestPartRefusedWhenBankOff(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: part("1000")})
	if !errors.Is(err, ErrPartNotAllowed) {
		t.Fatalf("err = %v, want ErrPartNotAllowed", err)
	}
	if n := f.count(t, `SELECT count(*) FROM case_disbursements WHERE case_id = $1`, c.ID); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
}

func TestInvalidPaymentKind(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	p := full("1000")
	p.Kind = "half"
	if _, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: p}); !errors.Is(err, ErrInvalidPaymentKind) {
		t.Fatalf("err = %v, want ErrInvalidPaymentKind", err)
	}
}

func TestFullPaymentCompletes(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	got, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: full("1000000")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %s, want completed", got.Status)
	}
}

func TestMoveBackAndReenterRecompletesWithoutPrompt(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	ctx := context.Background()
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: full("1000000")}); err != nil {
		t.Fatal(err)
	}
	back, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "documentation"), Remark: "by mistake"})
	if err != nil || back.Status != "active" {
		t.Fatalf("move back = %s, %v", back.Status, err)
	}
	again, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed")})
	if err != nil {
		t.Fatalf("re-enter without payment: %v", err)
	}
	if again.Status != "completed" {
		t.Fatalf("status = %s, want completed", again.Status)
	}
	if n := f.count(t, `SELECT count(*) FROM case_disbursements WHERE case_id = $1`, c.ID); n != 1 {
		t.Fatalf("payments = %d, want 1", n)
	}
	// A payment sent on re-entry after Full is refused, not doubled.
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: full("5")}); !errors.Is(err, ErrAlreadyFullyPaid) {
		t.Fatalf("second full err = %v, want ErrAlreadyFullyPaid", err)
	}
}

func TestReenterWithOnlyPartsNeedsNoPayment(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "3500000")
	ctx := context.Background()
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "disbursement"), Payment: part("1000000")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "signing")}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "disbursement")})
	if err != nil || got.Status != "active" {
		t.Fatalf("re-enter = %s, %v", got.Status, err)
	}
}

func TestFirstPaymentOnMoveIsAudited(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: full("1000000")}); err != nil {
		t.Fatal(err)
	}
	for _, e := range f.rec.entries {
		d, _ := e.Details.(map[string]any)
		if e.Action == "case.payment_added" && d["amount"] == "1000000" && d["kind"] == PaymentFull && d["paid_on"] == "2026-11-20" {
			return
		}
	}
	t.Fatalf("no case.payment_added audit for the move's payment: %+v", f.rec.entries)
}

func TestEditLoanAmountAfterSanctionMovesSanctioned(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	ctx := context.Background()
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: bankStage(t, f, bank, "sanctioned"),
		Sanction: &SanctionDetails{Amount: "4500000", RatePercent: "8.5"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: "4000000"}); err != nil {
		t.Fatal(err)
	}
	sum, err := f.svc.Payments(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Remaining != "4000000.00" {
		t.Fatalf("remaining = %s, want 4000000.00 (the amount shown on the case)", sum.Remaining)
	}
}
