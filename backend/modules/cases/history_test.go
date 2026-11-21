package cases

import (
	"context"
	"testing"
)

func TestHistoryListsStageEventsInOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("HIST1"), nil, nil); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "under_sanction"), nil, nil, nil); err != nil {
		t.Fatalf("move 2: %v", err)
	}
	events, err := f.svc.History(ctx, c.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	want := []string{"received", "logged_in", "under_sanction"}
	if len(events) != len(want) {
		t.Fatalf("events = %d, want %d", len(events), len(want))
	}
	for i, e := range events {
		if e.StageCode != want[i] {
			t.Fatalf("event %d = %s, want %s", i, e.StageCode, want[i])
		}
		if e.ReachedAt.IsZero() || e.ChangedBy == "" {
			t.Fatalf("event %d missing date or user: %+v", i, e)
		}
	}
}

func TestByCustomerReturnsAllCases(t *testing.T) {
	f := newFixture(t)
	newCase(t, f)
	newCase(t, f)
	list, err := f.svc.ByCustomer(context.Background(), f.customer)
	if err != nil {
		t.Fatalf("ByCustomer: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("cases = %d, want 2", len(list))
	}
}
