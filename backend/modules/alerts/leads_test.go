package alerts

import (
	"context"
	"testing"
)

func addLead(t *testing.T, f fixture, nextCall, status string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO leads (customer_id, project_id, assignee_id, next_call_date, status, converted_at, dropped_reason_id, dropped_at)
		SELECT $1, p.id, $2, NULLIF($3, '')::date, $4,
		       CASE WHEN $4 = 'converted' THEN now() END,
		       CASE WHEN $4 = 'dropped' THEN (SELECT id FROM dropdown_values WHERE value='Not eligible') END,
		       CASE WHEN $4 = 'dropped' THEN now() END
		FROM projects p WHERE p.name = 'Green Villas'
		RETURNING id`, f.customer, f.colleague.ID, nextCall, status), &id)
	return id
}

func TestLeadCallDueQueuedOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	addLead(t, f, "2026-11-17", "open")
	res, err := f.queue.RunFollowUps(ctx, at(17, 10))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.LeadDue != 1 || res.Due != 0 {
		t.Fatalf("result = %+v, want one lead due", res)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(17, 18)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE lead_id IS NOT NULL AND kind = $1`, KindFollowUpDue); n != 1 {
		t.Fatalf("lead due alerts = %d, want 1", n)
	}
}

func TestLeadCallNotQueuedForClosedLeads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	addLead(t, f, "2026-11-17", "converted")
	addLead(t, f, "2026-11-17", "dropped")
	if _, err := f.queue.RunFollowUps(ctx, at(17, 10)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log`); n != 0 {
		t.Fatalf("alerts for closed leads = %d", n)
	}
}

func TestLeadCallOverdueNextDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	addLead(t, f, "2026-11-23", "open")
	res, err := f.queue.RunFollowUps(ctx, at(24, 9))
	if err != nil {
		t.Fatal(err)
	}
	if res.LeadOverdue != 1 {
		t.Fatalf("result = %+v, want one lead overdue", res)
	}
}

func TestDispatchLeadAlertText(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	lead := addLead(t, f, "2026-11-17", "open")
	if _, err := f.queue.RunFollowUps(ctx, at(17, 10)); err != nil {
		t.Fatal(err)
	}
	if err := f.queue.QueueLeadAssigned(ctx, lead, f.colleague.ID); err != nil {
		t.Fatal(err)
	}
	s := &fakeSender{}
	res, err := f.queue.Dispatch(ctx, s, 10)
	if err != nil || res.Sent != 2 {
		t.Fatalf("dispatch: %v %+v", err, res)
	}
	want := map[string]string{
		"colleague@example.com|Call due today":                 "Call Ravi about Green Villas today.",
		"colleague@example.com|A lead has been assigned to you": "Lead for Ravi (Green Villas) has been assigned to you in Nexflow.",
	}
	for i, sent := range s.sent {
		if body, ok := want[sent]; !ok || body != s.bodies[i] {
			t.Errorf("sent %q body %q", sent, s.bodies[i])
		}
	}
}

func TestQueueLeadAssigned(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	lead := addLead(t, f, "", "open")
	if err := f.queue.QueueLeadAssigned(ctx, lead, f.colleague.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE lead_id = $1 AND case_id IS NULL AND kind = 'assigned'`, lead); n != 1 {
		t.Fatalf("lead assigned rows = %d", n)
	}
}

// A call date several days back still gets one overdue email.
func TestLeadCallOverdueCatchesOlderDates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (name) VALUES ('Green Villas')`); err != nil {
		t.Fatal(err)
	}
	addLead(t, f, "2026-11-20", "open")
	if _, err := f.queue.RunFollowUps(ctx, at(24, 9)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.queue.RunFollowUps(ctx, at(25, 9)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM alert_log WHERE lead_id IS NOT NULL AND kind = $1`, KindFollowUpOverdue); n != 1 {
		t.Fatalf("overdue alerts = %d, want 1", n)
	}
}
