package alerts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// addQuery adds a bank query on a case, raised on 10 Nov, assigned to actor (the case goes to colleague).
func addQuery(t *testing.T, f fixture, caseID int64, due, status string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO case_queries (case_id, number, body, raised_on, due_on, assignee_id, status, created_by, updated_by)
		VALUES ($1, (SELECT count(*) + 1 FROM case_queries WHERE case_id = $1), 'Salary slips', '2026-11-10', $2::date, $3, $4, $3, $3)`,
		caseID, due, f.actor.ID, status); err != nil {
		t.Fatal(err)
	}
}

func TestQueryDueAlertToQueryAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := addCase(t, f, "", "active")
	addQuery(t, f, c, "2026-11-17", "open")
	addQuery(t, f, c, "2026-11-17", "sent") // waiting on the bank: no alert
	res, err := f.queue.RunFollowUps(ctx, at(17, 10))
	if err != nil {
		t.Fatalf("RunFollowUps: %v", err)
	}
	if res.QueryDue != 1 || kindCount(t, f, KindQueryDue) != 1 {
		t.Fatalf("query due = %d / %d rows, want 1", res.QueryDue, kindCount(t, f, KindQueryDue))
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE kind = 'query_due' AND recipient_user_id = $1`, f.actor.ID); n != 1 {
		t.Fatalf("query due alerts to the query assignee = %d, want 1", n)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(17, 18)); err != nil {
		t.Fatal(err)
	}
	if n := kindCount(t, f, KindQueryDue); n != 1 {
		t.Fatalf("after second run = %d, want 1", n)
	}
}

func TestQueryOverdueAlertNextDayOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := addCase(t, f, "", "active")
	addQuery(t, f, c, "2026-11-16", "open")
	dropped := addCase(t, f, "", "dropped")
	addQuery(t, f, dropped, "2026-11-16", "open")
	res, err := f.queue.RunFollowUps(ctx, at(17, 10))
	if err != nil {
		t.Fatal(err)
	}
	if res.QueryOverdue != 1 || kindCount(t, f, KindQueryOverdue) != 1 {
		t.Fatalf("overdue = %d, want 1 (dropped case none)", res.QueryOverdue)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(18, 10)); err != nil {
		t.Fatal(err)
	}
	if n := kindCount(t, f, KindQueryOverdue); n != 1 {
		t.Fatalf("overdue after the next day = %d, want 1", n)
	}
}

func TestQueryAlertTexts(t *testing.T) {
	ref := pgtype.Int8{Int64: 42, Valid: true}
	s, b := compose(KindQueryDue, "Ravi", "", ref, pgtype.Int8{})
	if s != "Bank query due today" || b != "Case 42 for Ravi has a bank query due today." {
		t.Fatalf("due = %q / %q", s, b)
	}
	s, b = compose(KindQueryOverdue, "Ravi", "", ref, pgtype.Int8{})
	if s != "Bank query overdue" || b != "Case 42 for Ravi has a bank query that is overdue." {
		t.Fatalf("overdue = %q / %q", s, b)
	}
}

// Review fix: queries on completed (disbursed) cases still send alerts.
func TestQueryAlertOnCompletedCase(t *testing.T) {
	f := newFixture(t)
	c := addCase(t, f, "", "completed")
	addQuery(t, f, c, "2026-11-17", "open")
	if _, err := f.queue.RunFollowUps(context.Background(), at(17, 10)); err != nil {
		t.Fatal(err)
	}
	if n := kindCount(t, f, KindQueryDue); n != 1 {
		t.Fatalf("query due on completed case = %d, want 1", n)
	}
}

// A query reopened after its due date (bank rejected the proof) still gets one overdue alert.
func TestQueryOverdueAlertWhenReopenedLate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := addCase(t, f, "", "active")
	addQuery(t, f, c, "2026-11-12", "open")
	res, err := f.queue.RunFollowUps(ctx, at(17, 10))
	if err != nil {
		t.Fatal(err)
	}
	if res.QueryOverdue != 1 {
		t.Fatalf("overdue = %d, want 1 for a query due 5 days ago", res.QueryOverdue)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(18, 10)); err != nil {
		t.Fatal(err)
	}
	if n := kindCount(t, f, KindQueryOverdue); n != 1 {
		t.Fatalf("overdue rows = %d, want 1 (once only)", n)
	}
}
