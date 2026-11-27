package cases

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stageID returns the ID of a standard stage by code.
func stageID(t *testing.T, f fixture, code string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`SELECT id FROM stages WHERE code = $1 AND bank_id IS NULL`, code), &id)
	return id
}

// newCase opens a case for the fixture customer at Received.
func newCase(t *testing.T, f fixture) Case {
	t.Helper()
	c, err := f.svc.Create(context.Background(), f.actor, NewCase{
		ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "1000000", AssigneeID: f.actor.ID,
	})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	return c
}

func tracking(s string) *string { return &s }

func TestForwardMoveAddsDatedEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)

	before := time.Now().Add(-time.Second)
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("APP1001")})
	if err != nil {
		t.Fatalf("ChangeStage: %v", err)
	}
	if got.CurrentStageID != stageID(t, f, "logged_in") {
		t.Fatalf("current stage not updated: %d", got.CurrentStageID)
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); n != 2 {
		t.Fatalf("stage events = %d, want 2", n)
	}
	var reached time.Time
	mustScan(t, f.pool.QueryRow(ctx,
		`SELECT reached_at FROM case_stage_events WHERE case_id = $1 ORDER BY id DESC LIMIT 1`, c.ID), &reached)
	if reached.Before(before) {
		t.Fatalf("event date %v is before the move", reached)
	}
}

func TestBackwardMoveAllowedAndRecorded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("APP2001")}); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "under_sanction")}); err != nil {
		t.Fatalf("forward 2: %v", err)
	}
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in")})
	if err != nil {
		t.Fatalf("backward: %v", err)
	}
	if got.CurrentStageID != stageID(t, f, "logged_in") {
		t.Fatalf("backward move did not change stage")
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); n != 4 {
		t.Fatalf("stage events = %d, want 4 (every move recorded)", n)
	}
}

func TestMoveToLoggedInRequiresTrackingNumber(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in")})
	if !errors.Is(err, ErrTrackingRequired) {
		t.Fatalf("err = %v, want ErrTrackingRequired", err)
	}
	var cur int64
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT current_stage_id FROM cases WHERE id=$1`, c.ID), &cur)
	if cur != stageID(t, f, "received") {
		t.Fatal("refused move still changed the stage")
	}
}

func TestMoveToDisbursedSetsStatusCompleted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("APP3001")}); err != nil {
		t.Fatalf("logged_in: %v", err)
	}
	got, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "disbursed"), Payment: &PaymentInput{Amount: "1000000", Kind: "full"}})
	if err != nil {
		t.Fatalf("disbursed: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %s, want completed", got.Status)
	}
}

func TestStageHistoryNeverOverwritten(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("APP4001")}); err != nil {
		t.Fatalf("move 1: %v", err)
	}
	var firstAt time.Time
	mustScan(t, f.pool.QueryRow(ctx,
		`SELECT reached_at FROM case_stage_events WHERE case_id=$1 ORDER BY id LIMIT 1`, c.ID), &firstAt)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "under_sanction")}); err != nil {
		t.Fatalf("move 2: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id=$1`, c.ID); n != 3 {
		t.Fatalf("events = %d, want 3", n)
	}
	var stillFirst time.Time
	mustScan(t, f.pool.QueryRow(ctx,
		`SELECT reached_at FROM case_stage_events WHERE case_id=$1 ORDER BY id LIMIT 1`, c.ID), &stillFirst)
	if !stillFirst.Equal(firstAt) {
		t.Fatalf("earlier event changed: %v -> %v", firstAt, stillFirst)
	}
}

func TestEachStageChangeWritesAuditEntry(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("APP5001")}); err != nil {
		t.Fatalf("ChangeStage: %v", err)
	}
	for _, e := range f.rec.entries {
		if e.Action == "case.stage_changed" {
			return
		}
	}
	t.Fatalf("no case.stage_changed audit entry; got %+v", f.rec.entries)
}
