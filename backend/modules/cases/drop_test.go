package cases

import (
	"context"
	"errors"
	"testing"
)

// reasonID returns the dropdown ID of a drop reason by its text.
func reasonID(t *testing.T, f fixture, value string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`SELECT id FROM dropdown_values WHERE category='drop_reason' AND value=$1`, value), &id)
	return id
}

func TestDropRequiresReason(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	_, err := f.svc.Drop(context.Background(), f.actor, c.ID, DropInput{ReasonID: 0})
	if !errors.Is(err, ErrInvalidReason) {
		t.Fatalf("err = %v, want ErrInvalidReason", err)
	}
}

func TestDropOtherRequiresRemark(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	other := reasonID(t, f, "Other")
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: other, Remark: "  "}); !errors.Is(err, ErrRemarkRequired) {
		t.Fatalf("Other without remark err = %v, want ErrRemarkRequired", err)
	}
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: other, Remark: "Went abroad"}); err != nil {
		t.Fatalf("Other with remark: %v", err)
	}
}

func TestDroppedCaseStaysVisibleWithStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	got, err := f.svc.load(ctx, c.ID)
	if err != nil {
		t.Fatalf("case vanished after drop: %v", err)
	}
	if got.Status != "dropped" {
		t.Fatalf("status = %s, want dropped", got.Status)
	}
}

func TestDropKeepsStageHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("DRP1"), nil, nil); err != nil {
		t.Fatalf("stage: %v", err)
	}
	before := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id=$1`, c.ID)
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Customer unreachable")}); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if after := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id=$1`, c.ID); after != before {
		t.Fatalf("stage events %d -> %d after drop, want unchanged", before, after)
	}
}

// Owner answer A (round 13): a disbursed case is dropped too.
func TestDisbursedCaseCanBeDropped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("DRP2"), nil, nil); err != nil {
		t.Fatalf("logged_in: %v", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "disbursed"), nil, nil, nil); err != nil {
		t.Fatalf("disbursed: %v", err)
	}
	got, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Other"), Remark: "Closed after disbursement"})
	if err != nil {
		t.Fatalf("Drop disbursed: %v", err)
	}
	if got.Status != "dropped" || got.CurrentStageID != stageID(t, f, "disbursed") {
		t.Fatalf("after drop status=%s stage=%d, want dropped at disbursed", got.Status, got.CurrentStageID)
	}
}

func TestDroppedCaseCannotBeDroppedAgain(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatalf("first drop: %v", err)
	}
	_, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Other"), Remark: "x"})
	if !errors.Is(err, ErrAlreadyDropped) {
		t.Fatalf("second drop err = %v, want ErrAlreadyDropped", err)
	}
}
