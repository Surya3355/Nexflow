package alerts

import (
	"context"
	"fmt"
	"testing"
)

// queuedCase makes a case for the colleague and queues its assigned alert.
func queuedCase(t *testing.T, f fixture, mobile string) int64 {
	t.Helper()
	ctx := context.Background()
	var customer, caseID int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('C', $1) RETURNING id`, mobile), &customer)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		SELECT $1, $2, 1000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3 RETURNING id`,
		customer, f.bank, f.colleague.ID), &caseID)
	if err := f.queue.Enqueue(ctx, caseID, f.colleague.ID, KindAssigned, nil); err != nil {
		t.Fatal(err)
	}
	return caseID
}

func skipReason(t *testing.T, f fixture, caseID int64) string {
	t.Helper()
	var reason *string
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT skip_reason FROM alert_log WHERE case_id = $1`, caseID), &reason)
	if reason == nil {
		return ""
	}
	return *reason
}

func TestDispatchSkipsAfterFiveFailures(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := queuedCase(t, f, "9100000001")
	down := &fakeSender{fail: true}
	for i := 0; i < 5; i++ {
		if _, err := f.queue.Dispatch(ctx, down, 10); err != nil {
			t.Fatal(err)
		}
	}
	if got := skipReason(t, f, id); got != "failed 5 times" {
		t.Fatalf("skip reason = %q, want failed 5 times", got)
	}
	up := &fakeSender{}
	if _, err := f.queue.Dispatch(ctx, up, 10); err != nil {
		t.Fatal(err)
	}
	if len(up.sent) != 0 {
		t.Fatalf("skipped alert was sent: %v", up.sent)
	}
}

func TestDispatchSkipsOld(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := queuedCase(t, f, "9100000002")
	if _, err := f.pool.Exec(ctx, `UPDATE alert_log SET queued_at = now() - interval '3 days 1 minute' WHERE case_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	up := &fakeSender{}
	if _, err := f.queue.Dispatch(ctx, up, 10); err != nil {
		t.Fatal(err)
	}
	if len(up.sent) != 0 {
		t.Fatalf("old alert was sent: %v", up.sent)
	}
	if got := skipReason(t, f, id); got != "older than 3 days" {
		t.Fatalf("skip reason = %q", got)
	}
}

func TestDispatchSkipsNoEmail(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE users SET email = NULL WHERE id = $1`, f.colleague.ID); err != nil {
		t.Fatal(err)
	}
	id := queuedCase(t, f, "9100000003")
	if _, err := f.queue.Dispatch(ctx, &fakeSender{}, 10); err != nil {
		t.Fatal(err)
	}
	if got := skipReason(t, f, id); got != "no email address" {
		t.Fatalf("skip reason = %q", got)
	}
}

func TestFailingDoNotBlockNewer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		id := queuedCase(t, f, fmt.Sprintf("92000%05d", i))
		if _, err := f.pool.Exec(ctx, `UPDATE alert_log SET attempts = 3, queued_at = now() - interval '1 hour' WHERE case_id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	fresh := queuedCase(t, f, "9300000000")
	if _, err := f.queue.Dispatch(ctx, &fakeSender{}, 100); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE case_id = $1 AND sent_at IS NOT NULL`, fresh); n != 1 {
		t.Fatal("the new alert waited behind 100 retried ones")
	}
}
