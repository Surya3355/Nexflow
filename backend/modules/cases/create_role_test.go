package cases

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func stageCodeOf(t *testing.T, f fixture, c Case) string {
	t.Helper()
	var code string
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT code FROM stages WHERE id = $1`, c.CurrentStageID), &code)
	return code
}

func TestHDFCCaseStartsAtLinkSent(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "5000000")
	if code := stageCodeOf(t, f, c); code != "link_sent" {
		t.Fatalf("start stage = %s, want link_sent", code)
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); n != 1 {
		t.Fatalf("stage events = %d, want 1", n)
	}
}

func TestHDFCCaseTrackingStartRefused(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	for _, start := range []string{"tracking", "logged_in"} {
		_, err := f.svc.Create(context.Background(), f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: bank,
			LoanAmount: "100000", AssigneeID: f.actor.ID, StartAt: start, TrackingNumber: "HD1"})
		if !errors.Is(err, ErrInvalidStart) {
			t.Fatalf("start %s err = %v, want ErrInvalidStart", start, err)
		}
	}
}

func TestStandardStartChoicesUnchanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i, tc := range []struct{ start, want string }{
		{"", "received"}, {"start", "received"}, {"received", "received"},
		{"tracking", "logged_in"}, {"logged_in", "logged_in"},
	} {
		c, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID,
			LoanAmount: "100000", AssigneeID: f.actor.ID, StartAt: tc.start, TrackingNumber: fmt.Sprintf("STD%d", i)})
		if err != nil {
			t.Fatalf("start %q: %v", tc.start, err)
		}
		if code := stageCodeOf(t, f, c); code != tc.want {
			t.Errorf("start %q = %s, want %s", tc.start, code, tc.want)
		}
	}
}

func TestLeadToHDFCCaseStartsAtLinkSent(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	leadID, _ := f.openLead(t)
	c, err := f.svc.CreateFromLead(context.Background(), f.actor, leadID, FromLead{BankID: bank, LoanAmount: "4500000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if code := stageCodeOf(t, f, c); code != "link_sent" {
		t.Fatalf("start stage = %s, want link_sent", code)
	}
}

func TestPartPaidCaseNotStuck(t *testing.T) {
	f := newFixture(t)
	_, c := hdfcAtDisbursement(t, f, "3500000", "1000000")
	backdateStage(t, f, c.ID, 60)
	if _, ok := attentionFor(t, f, nil)[c.ID]; ok {
		t.Fatal("a part-paid case waiting at Disbursement should not be stuck")
	}
}
