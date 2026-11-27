package cases

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestDuplicateTrackingNumberRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := newCase(t, f)
	second := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, first.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("DUP100")}); err != nil {
		t.Fatalf("first case: %v", err)
	}
	_, err := f.svc.ChangeStage(ctx, f.actor, second.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("DUP100")})
	if !errors.Is(err, ErrDuplicateTracking) {
		t.Fatalf("duplicate err = %v, want ErrDuplicateTracking", err)
	}
}

func TestTrackingNumberIsTrimmed(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	got, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking(" 12345 ")})
	if err != nil {
		t.Fatalf("ChangeStage: %v", err)
	}
	if got.TrackingNumber == nil || *got.TrackingNumber != "12345" {
		t.Fatalf("stored tracking = %v, want 12345", got.TrackingNumber)
	}
}

// Two staff set the same tracking number at the same moment: exactly one
// succeeds, the other gets ErrDuplicateTracking, not a server error.
func TestConcurrentDuplicateTrackingOneSucceeds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newCase(t, f)
	b := newCase(t, f)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []int64{a.ID, b.ID} {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			_, errs[i] = f.svc.ChangeStage(ctx, f.actor, id, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("RACE1")})
		}(i, id)
	}
	wg.Wait()

	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrDuplicateTracking):
			dup++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 1 {
		t.Fatalf("successes=%d duplicates=%d, want 1 and 1", ok, dup)
	}
}
