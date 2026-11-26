package cases

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeTracking(t *testing.T) {
	ok := map[string]string{"hdfc123": "HDFC123", "  sbi9 ": "SBI9", "": "", "   ": "", "ABC": "ABC", "0042": "0042"}
	for in, want := range ok {
		got, err := NormalizeTracking(in)
		if err != nil || got != want {
			t.Errorf("NormalizeTracking(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"HDFC 123", "hdfc-123", "A/1", "१२३", "ab_12", "Ä1"} {
		if _, err := NormalizeTracking(in); !errors.Is(err, ErrInvalidTracking) {
			t.Errorf("NormalizeTracking(%q) err = %v, want ErrInvalidTracking", in, err)
		}
	}
	if ErrInvalidTracking.Error() != "cases: tracking number can contain only letters and numbers" {
		t.Errorf("message = %q", ErrInvalidTracking.Error())
	}
}

func TestLoggedInStoresUpperTracking(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	got, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("ab12")})
	if err != nil {
		t.Fatalf("ChangeStage: %v", err)
	}
	if got.TrackingNumber == nil || *got.TrackingNumber != "AB12" {
		t.Fatalf("stored = %v, want AB12", got.TrackingNumber)
	}
}

func TestLoggedInRefusesSymbols(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("ab-12")})
	if !errors.Is(err, ErrInvalidTracking) {
		t.Fatalf("err = %v, want ErrInvalidTracking", err)
	}
	got, _ := f.svc.Get(context.Background(), c.ID)
	if got.CurrentStageID == stageID(t, f, "logged_in") {
		t.Fatal("case moved despite invalid tracking number")
	}
}

func TestDuplicateTrackingIgnoresCase(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first, second := newCase(t, f), newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, first.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("AB12")}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, second.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("ab12")}); !errors.Is(err, ErrDuplicateTracking) {
		t.Fatalf("err = %v, want ErrDuplicateTracking", err)
	}
}

func TestSearchTrackingLowercase(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, StageMove{ToStageID: stageID(t, f, "logged_in"), TrackingNumber: tracking("AB12")}); err != nil {
		t.Fatalf("ChangeStage: %v", err)
	}
	groups, err := f.svc.Search(ctx, SearchQuery{Text: "ab12"})
	if err != nil || groupFor(groups, f.customer) == nil {
		t.Fatalf("search ab12 = %+v %v, want Ravi", groups, err)
	}
}
