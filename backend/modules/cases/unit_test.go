package cases

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeUnit(t *testing.T) {
	ok := map[string]string{" b-14 ": "B-14", "Flat 3/2": "FLAT 3/2", "": "", "  ": "", "12": "12"}
	for in, want := range ok {
		got, err := NormalizeUnit(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUnit(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"#12", "-12", strings.Repeat("A", 21), "B_14"} {
		if _, err := NormalizeUnit(in); !errors.Is(err, ErrInvalidUnit) {
			t.Errorf("NormalizeUnit(%q) err = %v, want ErrInvalidUnit", in, err)
		}
	}
}

func TestCreateCaseWithUnitAndLoggedIn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := NewCase{CustomerID: f.customer, BankID: f.bankID, ProjectID: f.naProject, LoanAmount: "100000",
		AssigneeID: f.actor.ID, UnitNumber: "a1", StartAt: "logged_in", TrackingNumber: "ab12"}
	c, err := f.svc.Create(ctx, f.actor, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.UnitNumber == nil || *c.UnitNumber != "A1" || c.TrackingNumber == nil || *c.TrackingNumber != "AB12" || c.LeadID != nil {
		t.Errorf("case = %+v", c)
	}
	in.StartAt, in.TrackingNumber = "sanctioned", ""
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrInvalidStart) {
		t.Errorf("bad start err = %v", err)
	}
	in.StartAt, in.UnitNumber = "", "#1"
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrInvalidUnit) {
		t.Errorf("bad unit err = %v", err)
	}
	in.UnitNumber, in.StartAt, in.TrackingNumber = "", "logged_in", "AB12"
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrDuplicateTracking) {
		t.Errorf("duplicate tracking err = %v", err)
	}
}

func TestSetUnitNumber(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, err := f.svc.Create(ctx, f.actor, NewCase{CustomerID: f.customer, BankID: f.bankID, ProjectID: f.naProject, LoanAmount: "100000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.SetUnitNumber(ctx, f.actor, c.ID, "c-7")
	if err != nil || got.UnitNumber == nil || *got.UnitNumber != "C-7" {
		t.Fatalf("set: %v %v", err, got.UnitNumber)
	}
	got, err = f.svc.SetUnitNumber(ctx, f.actor, c.ID, "")
	if err != nil || got.UnitNumber != nil {
		t.Fatalf("clear: %v %v", err, got.UnitNumber)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE cases SET status='dropped', dropped_at=now(),
		dropped_reason_id=(SELECT id FROM dropdown_values WHERE category='drop_reason' LIMIT 1) WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetUnitNumber(ctx, f.actor, c.ID, "X"); !errors.Is(err, ErrCaseDropped) {
		t.Errorf("dropped err = %v", err)
	}
}

func TestCreateCaseNeedsBankAndActiveAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := NewCase{CustomerID: f.customer, BankID: 0, ProjectID: f.naProject, LoanAmount: "100000", AssigneeID: f.actor.ID}
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrBankRequired) {
		t.Errorf("no bank err = %v, want ErrBankRequired", err)
	}
	in.BankID = 99999
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrBankRequired) {
		t.Errorf("unknown bank err = %v, want ErrBankRequired", err)
	}
	in.BankID, in.AssigneeID = f.bankID, 0
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrAssigneeInactive) {
		t.Errorf("no assignee err = %v, want ErrAssigneeInactive", err)
	}
	leadID, _ := f.openLead(t)
	if _, err := f.svc.CreateFromLead(ctx, f.actor, leadID, FromLead{LoanAmount: "100000", AssigneeID: f.actor.ID}); !errors.Is(err, ErrBankRequired) {
		t.Errorf("from lead without bank err = %v, want ErrBankRequired", err)
	}
}
