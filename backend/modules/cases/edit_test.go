package cases

import (
	"context"
	"testing"
	"time"
)

func TestFollowUpDateOptional(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)

	got, err := f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: "1000000", FollowUpDate: nil})
	if err != nil {
		t.Fatalf("Update without date: %v", err)
	}
	if got.FollowUpDate != nil {
		t.Fatalf("follow-up date = %v, want none", got.FollowUpDate)
	}
	var isNull bool
	mustScan(t, f.pool.QueryRow(ctx, `SELECT follow_up_date IS NULL FROM cases WHERE id=$1`, c.ID), &isNull)
	if !isNull {
		t.Fatal("follow_up_date stored as a value, want NULL")
	}

	day := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	got, err = f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: "1000000", FollowUpDate: &day})
	if err != nil {
		t.Fatalf("Update with date: %v", err)
	}
	if got.FollowUpDate == nil || !got.FollowUpDate.Equal(day) {
		t.Fatalf("follow-up date = %v, want %v", got.FollowUpDate, day)
	}
}

func TestAllStaffCanEditCase(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.svc.Update(context.Background(), f.actor, c.ID, CaseFields{LoanAmount: "1200000"}); err != nil {
		t.Fatalf("employee edit: %v", err)
	}
}

func TestEditDoesNotChangeStageOrAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("EDIT1"), nil, nil); err != nil {
		t.Fatalf("stage: %v", err)
	}
	before, err := f.svc.load(ctx, c.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := f.svc.Update(ctx, f.actor, c.ID, CaseFields{LoanAmount: "2000000"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, err := f.svc.load(ctx, c.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.CurrentStageID != before.CurrentStageID || after.AssigneeID != before.AssigneeID || after.Status != before.Status {
		t.Fatalf("edit changed stage/assignee/status: before=%+v after=%+v", before, after)
	}
}
