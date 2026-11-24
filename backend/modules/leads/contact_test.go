package leads

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLogContactUpdatesNextCall(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	next := day(2026, 11, 25)
	got, err := f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "call_back", Remark: " Busy, call Monday ", NextCallDate: &next})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if got.NextCallDate == nil || !got.NextCallDate.Equal(next) {
		t.Errorf("next call = %v, want %v", got.NextCallDate, next)
	}
	got, err = f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "interested", Remark: "Coming tomorrow"})
	if err != nil {
		t.Fatalf("second log: %v", err)
	}
	if got.NextCallDate != nil {
		t.Errorf("next call = %v, want none", got.NextCallDate)
	}
	cs, err := f.svc.Contacts(ctx, l.ID)
	if err != nil || len(cs) != 2 {
		t.Fatalf("contacts: %v, %d", err, len(cs))
	}
	if cs[0].Response != "interested" || cs[1].Remark != "Busy, call Monday" || cs[1].CreatedBy != "Staff One" {
		t.Errorf("contacts = %+v", cs)
	}
	if !f.rec.has("lead.contacted") {
		t.Error("no lead.contacted audit")
	}
	row, _ := f.svc.Get(ctx, l.ID, testNow)
	if row.LastResponse != "interested" || row.LastContactAt == nil {
		t.Errorf("last contact = %q %v", row.LastResponse, row.LastContactAt)
	}
}

func TestLogContactNotInterestedKeepsLeadOpen(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9845011212", f.project)
	got, err := f.svc.LogContact(context.Background(), f.actor, l.ID, ContactIn{Response: "not_interested", Remark: "Bought elsewhere"})
	if err != nil || got.Status != StatusOpen {
		t.Fatalf("log: %v, status %s", err, got.Status)
	}
}

func TestLogContactPastDateAccepted(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9845011212", f.project)
	past := day(2026, 11, 20)
	got, err := f.svc.LogContact(context.Background(), f.actor, l.ID, ContactIn{Response: "call_back", Remark: "Logged late", NextCallDate: &past})
	if err != nil || got.NextCallDate == nil || !got.NextCallDate.Equal(past) {
		t.Fatalf("log: %v, next %v", err, got.NextCallDate)
	}
}

func TestLogContactValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "maybe", Remark: "x"}); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("bad response err = %v", err)
	}
	if _, err := f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "interested", Remark: "   "}); !errors.Is(err, ErrRemarkRequired) {
		t.Errorf("empty remark err = %v", err)
	}
	if _, err := f.svc.Drop(ctx, f.actor, l.ID, f.reason, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "interested", Remark: "x"}); !errors.Is(err, ErrLeadClosed) {
		t.Errorf("dropped lead err = %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM lead_contacts`); n != 0 {
		t.Errorf("contacts stored = %d", n)
	}
}

func dueAlerts(t *testing.T, f fixture, leadID int64) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM alert_log WHERE lead_id = $1 AND kind = 'follow_up_due'`, leadID)
}

// The nightly job runs at 02:00 IST, so a call due today that is set during
// the day queues its reminder at once (spec §8 call 2).
func TestCallDueTodayQueuesReminderAtOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	var due time.Time
	mustScan(t, f.pool.QueryRow(ctx, `SELECT due_on FROM alert_log WHERE lead_id = $1 AND kind = 'follow_up_due' AND recipient_user_id = $2`, l.ID, f.actor.ID), &due)
	if !due.Equal(day(2026, 11, 24)) {
		t.Fatalf("due_on = %v, want IST today 2026-11-24", due)
	}
	later := day(2026, 11, 26)
	if _, err := f.svc.LogContact(ctx, f.actor, l.ID, ContactIn{Response: "call_back", Remark: "later", NextCallDate: &later}); err != nil {
		t.Fatal(err)
	}
	if n := dueAlerts(t, f, l.ID); n != 1 {
		t.Errorf("due alerts after a future call = %d, want 1", n)
	}
}

func TestContactWithCallTodayQueuesReminder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := f.newLead(t, "9845011212", f.project)
	if _, err := f.pool.Exec(ctx, `DELETE FROM alert_log`); err != nil {
		t.Fatal(err)
	}
	today := day(2026, 11, 24)
	if _, err := f.svc.LogContactAt(ctx, f.actor, l.ID, ContactIn{Response: "call_back", Remark: "evening", NextCallDate: &today}, testNow); err != nil {
		t.Fatal(err)
	}
	if n := dueAlerts(t, f, l.ID); n != 1 {
		t.Errorf("due alerts = %d, want 1", n)
	}
}
