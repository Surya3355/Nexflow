package alerts

import (
	"context"
	"testing"
	"time"
)

// at returns a time on 17 Nov 2026 IST at the given hour.
func at(day, hour int) time.Time {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	return time.Date(2026, 11, day, hour, 0, 0, 0, ist)
}

func addCase(t *testing.T, f fixture, followUp string, status string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, follow_up_date, status, dropped_reason_id, dropped_at)
		SELECT $1, $2, 1000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3,
		       NULLIF($4, '')::date, $5,
		       CASE WHEN $5 = 'dropped' THEN (SELECT id FROM dropdown_values WHERE value='Not eligible') END,
		       CASE WHEN $5 = 'dropped' THEN now() END
		RETURNING id`, f.customer, f.bank, f.colleague.ID, followUp, status), &id)
	return id
}

func kindCount(t *testing.T, f fixture, kind string) int {
	t.Helper()
	return count(t, f.pool, `SELECT count(*) FROM alert_log WHERE kind=$1`, kind)
}

func TestNoDateNoAlert(t *testing.T) {
	f := newFixture(t)
	addCase(t, f, "", "active")
	if _, err := f.queue.RunFollowUps(context.Background(), at(17, 10)); err != nil {
		t.Fatalf("RunFollowUps: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log`); n != 0 {
		t.Fatalf("alerts for a case with no follow-up date = %d, want 0", n)
	}
}

func TestDueAlertOnDueDate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	addCase(t, f, "2026-11-17", "active")
	if _, err := f.queue.RunFollowUps(ctx, at(17, 10)); err != nil {
		t.Fatalf("RunFollowUps: %v", err)
	}
	if n := kindCount(t, f, KindFollowUpDue); n != 1 {
		t.Fatalf("due alerts on the due date = %d, want 1", n)
	}
	// Running again the same day does not add a second alert.
	if _, err := f.queue.RunFollowUps(ctx, at(17, 18)); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if n := kindCount(t, f, KindFollowUpDue); n != 1 {
		t.Fatalf("due alerts after a second run = %d, want 1", n)
	}
}

// Open point C (owner answer C): the overdue alert goes once, on the day after.
func TestOverdueAlertOnceOnNextDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	addCase(t, f, "2026-11-16", "active")

	if _, err := f.queue.RunFollowUps(ctx, at(16, 10)); err != nil {
		t.Fatalf("on the due date: %v", err)
	}
	if n := kindCount(t, f, KindFollowUpOverdue); n != 0 {
		t.Fatalf("overdue alert on the due date itself = %d, want 0", n)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(17, 10)); err != nil {
		t.Fatalf("day after: %v", err)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(17, 20)); err != nil {
		t.Fatalf("day after, later run: %v", err)
	}
	if n := kindCount(t, f, KindFollowUpOverdue); n != 1 {
		t.Fatalf("overdue alerts = %d, want exactly 1", n)
	}
}

func TestDroppedCaseGetsNoFollowUpAlert(t *testing.T) {
	f := newFixture(t)
	addCase(t, f, "2026-11-17", "dropped")
	if _, err := f.queue.RunFollowUps(context.Background(), at(17, 10)); err != nil {
		t.Fatalf("RunFollowUps: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log`); n != 0 {
		t.Fatalf("dropped case got %d follow-up alerts, want 0", n)
	}
}
