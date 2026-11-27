package cases

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/leads"
)

// openLead inserts an open lead for the fixture customer on a new project.
func (f fixture) openLead(t *testing.T) (leadID, projectID int64) {
	t.Helper()
	ctx := context.Background()
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO projects (name) VALUES ('Green Villas') RETURNING id`), &projectID)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id, next_call_date)
		VALUES ($1, $2, $3, current_date) RETURNING id`, f.customer, projectID, f.actor.ID), &leadID)
	return leadID, projectID
}

func TestCreateFromLeadAtReceived(t *testing.T) {
	f := newFixture(t)
	leadID, projectID := f.openLead(t)
	c, err := f.svc.CreateFromLead(context.Background(), f.actor, leadID,
		FromLead{BankID: f.bankID, LoanAmount: "4500000", AssigneeID: f.actor.ID, UnitNumber: "b-14 "})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ProjectID != projectID || c.CustomerID != f.customer || c.LeadID == nil || *c.LeadID != leadID {
		t.Errorf("case = %+v", c)
	}
	if c.UnitNumber == nil || *c.UnitNumber != "B-14" {
		t.Errorf("unit = %v, want B-14", c.UnitNumber)
	}
	if n := f.count(t, `SELECT count(*) FROM leads WHERE id = $1 AND status = 'converted' AND converted_at IS NOT NULL AND next_call_date IS NULL`, leadID); n != 1 {
		t.Error("lead not converted")
	}
	if n := f.count(t, `SELECT count(*) FROM case_stage_events WHERE case_id = $1`, c.ID); n != 1 {
		t.Errorf("stage events = %d, want 1", n)
	}
	got := map[string]bool{}
	for _, e := range f.rec.entries {
		got[e.Action] = true
	}
	if !got["case.created"] || !got["lead.converted"] {
		t.Errorf("audit = %v", got)
	}
}

func TestCreateFromLeadAtLoggedIn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	leadID, _ := f.openLead(t)
	in := FromLead{BankID: f.bankID, LoanAmount: "4500000", AssigneeID: f.actor.ID, StartAt: "logged_in"}
	if _, err := f.svc.CreateFromLead(ctx, f.actor, leadID, in); !errors.Is(err, ErrTrackingRequired) {
		t.Fatalf("no tracking err = %v", err)
	}
	in.TrackingNumber = "hl123"
	c, err := f.svc.CreateFromLead(ctx, f.actor, leadID, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.TrackingNumber == nil || *c.TrackingNumber != "HL123" {
		t.Errorf("tracking = %v", c.TrackingNumber)
	}
	var codes []string
	rows, err := f.pool.Query(ctx, `SELECT s.code FROM case_stage_events e JOIN stages s ON s.id = e.stage_id WHERE e.case_id = $1 ORDER BY e.reached_at, e.id`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code string
		_ = rows.Scan(&code)
		codes = append(codes, code)
	}
	if len(codes) != 2 || codes[0] != "received" || codes[1] != "logged_in" {
		t.Errorf("stage events = %v", codes)
	}
	var current string
	mustScan(t, f.pool.QueryRow(ctx, `SELECT s.code FROM cases c JOIN stages s ON s.id = c.current_stage_id WHERE c.id = $1`, c.ID), &current)
	if current != "logged_in" {
		t.Errorf("current stage = %s", current)
	}
}

func TestCreateFromLeadTwiceRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	leadID, _ := f.openLead(t)
	in := FromLead{BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID}
	if _, err := f.svc.CreateFromLead(ctx, f.actor, leadID, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateFromLead(ctx, f.actor, leadID, in); !errors.Is(err, leads.ErrLeadClosed) {
		t.Fatalf("second err = %v, want ErrLeadClosed", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cases`); n != 1 {
		t.Errorf("cases = %d", n)
	}
}

func TestCreateFromLeadConcurrent(t *testing.T) {
	f := newFixture(t)
	leadID, _ := f.openLead(t)
	in := FromLead{BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.svc.CreateFromLead(context.Background(), f.actor, leadID, in)
		}(i)
	}
	wg.Wait()
	ok, closed := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, leads.ErrLeadClosed):
			closed++
		default:
			t.Errorf("unexpected err: %v", err)
		}
	}
	if ok != 1 || closed != 1 {
		t.Errorf("ok=%d closed=%d", ok, closed)
	}
	if n := f.count(t, `SELECT count(*) FROM cases`); n != 1 {
		t.Errorf("cases = %d", n)
	}
}

func TestCreateFromDroppedLeadRefused(t *testing.T) {
	f := newFixture(t)
	leadID, _ := f.openLead(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE leads SET status='dropped', dropped_at=now(),
		dropped_reason_id=(SELECT id FROM dropdown_values WHERE category='drop_reason' LIMIT 1) WHERE id=$1`, leadID); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateFromLead(context.Background(), f.actor, leadID, FromLead{BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID})
	if !errors.Is(err, leads.ErrLeadClosed) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.svc.CreateFromLead(context.Background(), f.actor, 99999, FromLead{BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID}); !errors.Is(err, leads.ErrLeadNotFound) {
		t.Errorf("unknown lead err = %v", err)
	}
}

// If a case already points at an open lead (left over from a restore), Add
// case reports the lead as closed instead of failing with a server error.
func TestCreateFromLeadAlreadyLinkedIsClosed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	leadID, _ := f.openLead(t)
	if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, lead_id)
		VALUES ($1, $2, 1, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3, $4)`, f.customer, f.bankID, f.actor.ID, leadID); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateFromLead(ctx, f.actor, leadID, FromLead{BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID, StartAt: "logged_in", TrackingNumber: "ZZ1"})
	if !errors.Is(err, leads.ErrLeadClosed) {
		t.Fatalf("err = %v, want ErrLeadClosed", err)
	}
}
