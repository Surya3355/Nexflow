package leads

import (
	"context"
	"errors"
	"testing"
)

// addContact inserts a builder contact on a project and returns its id.
func (f fixture) addContact(t *testing.T, project int64, phone string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO project_contacts (project_id, name, phone, label) VALUES ($1, 'Ravi', $2, 'Sales') RETURNING id`, project, phone), &id)
	return id
}

func (f fixture) removeContact(t *testing.T, id int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE project_contacts SET removed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func TestNewLeadGetsFirstActiveContact(t *testing.T) {
	f := newFixture(t)
	first := f.addContact(t, f.project, "9000000101")
	second := f.addContact(t, f.project, "9000000102")
	f.removeContact(t, first)
	l := f.newLead(t, "9800000001", f.project)
	if l.BuilderContactID == nil || *l.BuilderContactID != second {
		t.Fatalf("contact = %v, want %d", l.BuilderContactID, second)
	}
}

func TestAddForCustomerGetsFirstActiveContact(t *testing.T) {
	f := newFixture(t)
	c := f.addContact(t, f.project, "9000000101")
	l := f.newLead(t, "9800000001", f.naProject)
	got, err := f.svc.AddForCustomer(context.Background(), f.actor, l.CustomerID, f.project, f.actor.ID, false, testNow)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.BuilderContactID == nil || *got.BuilderContactID != c {
		t.Fatalf("contact = %v, want %d", got.BuilderContactID, c)
	}
}

func TestNewLeadNAHasNoContact(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9800000001", f.naProject)
	if l.BuilderContactID != nil {
		t.Fatalf("NA lead contact = %v, want none", *l.BuilderContactID)
	}
}

func TestNewLeadOldProjectNoContact(t *testing.T) {
	f := newFixture(t)
	l := f.newLead(t, "9800000001", f.project)
	if l.BuilderContactID != nil {
		t.Fatalf("contact = %v, want none for a project with no contacts", *l.BuilderContactID)
	}
}

func TestSetBuilderContact(t *testing.T) {
	f := newFixture(t)
	a := f.addContact(t, f.project, "9000000101")
	b := f.addContact(t, f.project, "9000000102")
	l := f.newLead(t, "9800000001", f.project)
	got, err := f.svc.SetBuilderContact(context.Background(), f.actor, l.ID, b)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if got.BuilderContactID == nil || *got.BuilderContactID != b {
		t.Fatalf("contact = %v, want %d", got.BuilderContactID, b)
	}
	var found bool
	for _, e := range f.rec.entries {
		if e.Action == "lead.builder_contact_changed" {
			d, _ := e.Details.(map[string]any)
			if d["from"] == a && d["to"] == b {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no audit with from=%d to=%d: %+v", a, b, f.rec.entries)
	}
}

func TestSetBuilderContactRefusesRemovedOrForeign(t *testing.T) {
	f := newFixture(t)
	a := f.addContact(t, f.project, "9000000101")
	gone := f.addContact(t, f.project, "9000000102")
	f.removeContact(t, gone)
	var p2 int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO projects (name) VALUES ('Other') RETURNING id`), &p2)
	foreign := f.addContact(t, p2, "9000000103")
	l := f.newLead(t, "9800000001", f.project)
	for _, id := range []int64{gone, foreign, 999999} {
		if _, err := f.svc.SetBuilderContact(context.Background(), f.actor, l.ID, id); !errors.Is(err, ErrBuilderContactInvalid) {
			t.Errorf("contact %d err = %v, want ErrBuilderContactInvalid", id, err)
		}
	}
	got, _ := f.svc.Get(context.Background(), l.ID, testNow)
	if got.BuilderContactID == nil || *got.BuilderContactID != a {
		t.Fatalf("contact changed to %v", got.BuilderContactID)
	}
}

func TestSetBuilderContactClosedLead(t *testing.T) {
	f := newFixture(t)
	f.addContact(t, f.project, "9000000101")
	b := f.addContact(t, f.project, "9000000102")
	l := f.newLead(t, "9800000001", f.project)
	if _, err := f.svc.Drop(context.Background(), f.actor, l.ID, f.reason, "no"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := f.svc.SetBuilderContact(context.Background(), f.actor, l.ID, b); !errors.Is(err, ErrLeadClosed) {
		t.Fatalf("err = %v, want ErrLeadClosed", err)
	}
}

func TestLeadRowShowsRemovedContact(t *testing.T) {
	f := newFixture(t)
	a := f.addContact(t, f.project, "9000000101")
	l := f.newLead(t, "9800000001", f.project)
	f.removeContact(t, a)
	r, err := f.svc.Get(context.Background(), l.ID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if r.BuilderContact == nil || r.BuilderContact.ID != a || !r.BuilderContact.Removed ||
		r.BuilderContact.Phone != "9000000101" || r.BuilderContact.Label != "Sales" || r.BuilderContact.Name != "Ravi" {
		t.Fatalf("row contact = %+v", r.BuilderContact)
	}
	rows, _ := f.svc.ListOpen(context.Background(), testNow)
	if len(rows) != 1 || rows[0].BuilderContact == nil {
		t.Fatalf("list rows contact missing: %+v", rows)
	}
}
