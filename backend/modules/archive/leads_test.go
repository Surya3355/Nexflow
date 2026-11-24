package archive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// clearCustomer drops the fixture customer and archives its case, ready for delete.
func (f fixture) clearCustomer(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("delete case: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE customers SET status='dropped', dropped_at=now(),
		dropped_reason_id=(SELECT id FROM dropdown_values WHERE category='drop_reason' ORDER BY sort_order LIMIT 1) WHERE id=$1`, f.customer); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) addLead(t *testing.T, status string) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO leads (customer_id, project_id, assignee_id, status, dropped_reason_id, dropped_at)
		VALUES ($1, na_project_id(), $2, $3,
		CASE WHEN $3 = 'dropped' THEN (SELECT id FROM dropdown_values WHERE category='drop_reason' ORDER BY sort_order LIMIT 1) END,
		CASE WHEN $3 = 'dropped' THEN now() END) RETURNING id`, f.customer, f.employee.ID, status).Scan(&id); err != nil {
		t.Fatalf("lead: %v", err)
	}
	return id
}

func TestDeleteCustomerWithOpenLeadRefused(t *testing.T) {
	f := newFixture(t)
	f.clearCustomer(t)
	f.addLead(t, "open")
	if err := f.svc.DeleteCustomer(context.Background(), f.manager, f.customer); !errors.Is(err, ErrCustomerHasOpenLeads) {
		t.Fatalf("err = %v, want ErrCustomerHasOpenLeads", err)
	}
}

func TestDeleteCustomerArchivesLeadsAndContacts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.clearCustomer(t)
	lead := f.addLead(t, "dropped")
	for _, r := range []string{"first", "second"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO lead_contacts (lead_id, response, remark) VALUES ($1, 'call_back', $2)`, lead, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO alert_log (lead_id, recipient_user_id, kind) VALUES ($1, $2, 'assigned')`, lead, f.employee.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var snap string
	if err := f.pool.QueryRow(ctx, `SELECT snapshot::text FROM archive WHERE entity_type='customer' AND entity_id=$1`, f.customer).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Leads []struct {
			ID       int64             `json:"id"`
			Contacts []json.RawMessage `json:"contacts"`
		} `json:"leads"`
	}
	if err := json.Unmarshal([]byte(snap), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Leads) != 1 || doc.Leads[0].ID != lead || len(doc.Leads[0].Contacts) != 2 {
		t.Fatalf("snapshot leads = %+v", doc.Leads)
	}
	for _, q := range []string{`SELECT count(*) FROM leads`, `SELECT count(*) FROM lead_contacts`, `SELECT count(*) FROM alert_log WHERE lead_id IS NOT NULL`} {
		if n := count(t, f.pool, q); n != 0 {
			t.Errorf("%s = %d", q, n)
		}
	}
}

func TestRestoreCustomerBringsLeadsBackDropped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.clearCustomer(t)
	lead := f.addLead(t, "dropped")
	var p2, conv int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO projects (name) VALUES ('P2') RETURNING id`).Scan(&p2); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id, status, converted_at)
		VALUES ($1, $2, $3, 'converted', now()) RETURNING id`, f.customer, p2, f.employee.ID).Scan(&conv); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO lead_contacts (lead_id, response, remark) VALUES ($1, 'interested', 'x')`, lead); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatalf("delete: %v", err)
	}
	admin := seedUser(t, f.pool, "boss", "admin")
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "customer", f.customer), DropInput{}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM leads WHERE id IN ($1, $2) AND status = 'dropped' AND dropped_reason_id IS NOT NULL`, lead, conv); n != 2 {
		t.Errorf("restored dropped leads = %d, want 2", n)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM lead_contacts WHERE lead_id = $1`, lead); n != 1 {
		t.Errorf("restored contacts = %d", n)
	}
}

func TestRestoreOldCustomerSnapshotWithoutLeads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.clearCustomer(t)
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET snapshot = snapshot - 'leads' WHERE entity_type='customer'`); err != nil {
		t.Fatal(err)
	}
	admin := seedUser(t, f.pool, "boss", "admin")
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "customer", f.customer), DropInput{}); err != nil {
		t.Fatalf("restore old snapshot: %v", err)
	}
}

func TestRestoreCaseKeepsOrClearsLeadID(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		setup    func(f fixture, lead int64)
		keepLead bool
	}{
		{"lead unlinked", func(fixture, int64) {}, true},
		{"lead taken by another case", func(f fixture, lead int64) {
			var bank, stage int64
			_ = f.pool.QueryRow(ctx, `SELECT id FROM banks LIMIT 1`).Scan(&bank)
			_ = f.pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`).Scan(&stage)
			if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, lead_id)
				VALUES ($1, $2, 1, $3, $4, $5)`, f.customer, bank, stage, f.employee.ID, lead); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"lead gone", func(f fixture, lead int64) {
			if _, err := f.pool.Exec(ctx, `DELETE FROM leads WHERE id = $1`, lead); err != nil {
				t.Fatal(err)
			}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			var lead int64
			if err := f.pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id, status, converted_at)
				VALUES ($1, na_project_id(), $2, 'converted', now()) RETURNING id`, f.customer, f.employee.ID).Scan(&lead); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE cases SET lead_id = $2 WHERE id = $1`, f.caseID, lead); err != nil {
				t.Fatal(err)
			}
			if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
				t.Fatalf("delete: %v", err)
			}
			tc.setup(f, lead)
			admin := seedUser(t, f.pool, "boss", "admin")
			if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Customer not interested")); err != nil {
				t.Fatalf("restore: %v", err)
			}
			n := count(t, f.pool, `SELECT count(*) FROM cases WHERE id = $1 AND lead_id = $2`, f.caseID, lead)
			if (n == 1) != tc.keepLead {
				t.Errorf("lead kept = %v, want %v", n == 1, tc.keepLead)
			}
		})
	}
}

// A lead being added (customer row locked, lead not yet committed) when the
// delete starts must make the delete refuse, not lose the lead or fail with 500.
func TestDeleteCustomerWaitsForLeadBeingAdded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.clearCustomer(t)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM customers WHERE id = $1 FOR UPDATE`, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE customers SET status = 'active', dropped_reason_id = NULL, dropped_at = NULL WHERE id = $1`, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, na_project_id(), $2)`, f.customer, f.employee.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.svc.DeleteCustomer(ctx, f.manager, f.customer) }()
	time.Sleep(300 * time.Millisecond)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrCustomerHasOpenLeads) {
		t.Fatalf("delete err = %v, want ErrCustomerHasOpenLeads", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM leads WHERE customer_id = $1 AND status = 'open'`, f.customer); n != 1 {
		t.Fatalf("open leads after refused delete = %d, want 1", n)
	}
}
