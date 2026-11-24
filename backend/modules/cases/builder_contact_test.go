package cases

import (
	"context"
	"errors"
	"testing"
)

func addContact(t *testing.T, f fixture, project int64, phone string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO project_contacts (project_id, name, phone, label) VALUES ($1, 'Ravi', $2, 'Sales') RETURNING id`, project, phone), &id)
	return id
}

func removeContact(t *testing.T, f fixture, id int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE project_contacts SET removed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func caseOn(f fixture, project, contact int64) NewCase {
	return NewCase{ProjectID: project, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "1000000", AssigneeID: f.actor.ID, BuilderContactID: contact}
}

func TestCreateCaseNeedsContactWhenProjectHasOne(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	c := addContact(t, f, p, "9000000101")
	if _, err := f.svc.Create(ctx, f.actor, caseOn(f, p, 0)); !errors.Is(err, ErrBuilderContactRequired) {
		t.Fatalf("no contact err = %v, want ErrBuilderContactRequired", err)
	}
	got, err := f.svc.Create(ctx, f.actor, caseOn(f, p, c))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.BuilderContactID == nil || *got.BuilderContactID != c || got.BuilderContact == nil || got.BuilderContact.Phone != "9000000101" {
		t.Fatalf("contact = %v %+v", got.BuilderContactID, got.BuilderContact)
	}
}

func TestCreateCaseNAContactRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	c := addContact(t, f, p, "9000000101")
	if _, err := f.svc.Create(ctx, f.actor, caseOn(f, f.naProject, c)); !errors.Is(err, ErrBuilderContactInvalid) {
		t.Fatalf("NA with contact err = %v, want ErrBuilderContactInvalid", err)
	}
	got, err := f.svc.Create(ctx, f.actor, caseOn(f, f.naProject, 0))
	if err != nil || got.BuilderContactID != nil {
		t.Fatalf("NA case = %v, %v", got.BuilderContactID, err)
	}
	old := addProject(t, f, "Old")
	if got, err := f.svc.Create(ctx, f.actor, caseOn(f, old, 0)); err != nil || got.BuilderContactID != nil {
		t.Fatalf("old project case = %v, %v", got.BuilderContactID, err)
	}
}

func TestCreateCaseRemovedContactRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	gone := addContact(t, f, p, "9000000101")
	addContact(t, f, p, "9000000102")
	removeContact(t, f, gone)
	other := addProject(t, f, "Other")
	foreign := addContact(t, f, other, "9000000103")
	for _, id := range []int64{gone, foreign} {
		if _, err := f.svc.Create(ctx, f.actor, caseOn(f, p, id)); !errors.Is(err, ErrBuilderContactInvalid) {
			t.Errorf("contact %d err = %v, want ErrBuilderContactInvalid", id, err)
		}
	}
}

func TestFromLeadStoresContact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	addContact(t, f, p, "9000000101")
	second := addContact(t, f, p, "9000000102")
	var lead int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, $2, $3) RETURNING id`, f.customer, p, f.actor.ID), &lead)
	got, err := f.svc.CreateFromLead(ctx, f.actor, lead, FromLead{BankID: f.bankID, LoanAmount: "500000", AssigneeID: f.actor.ID, BuilderContactID: second})
	if err != nil {
		t.Fatalf("from lead: %v", err)
	}
	if got.BuilderContactID == nil || *got.BuilderContactID != second {
		t.Fatalf("contact = %v, want %d", got.BuilderContactID, second)
	}
}

func TestChangeProjectResetsContact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := addProject(t, f, "A")
	ca := addContact(t, f, a, "9000000101")
	b := addProject(t, f, "B")
	gone := addContact(t, f, b, "9000000102")
	cb := addContact(t, f, b, "9000000103")
	removeContact(t, f, gone)
	old := addProject(t, f, "Old")
	c, err := f.svc.Create(ctx, f.actor, caseOn(f, a, ca))
	if err != nil {
		t.Fatal(err)
	}
	before := len(f.rec.entries)
	got, err := f.svc.ChangeProject(ctx, f.actor, c.ID, b)
	if err != nil || got.BuilderContactID == nil || *got.BuilderContactID != cb {
		t.Fatalf("to B: contact = %v, %v; want %d", got.BuilderContactID, err, cb)
	}
	d, _ := f.rec.entries[before].Details.(map[string]any)
	if d["from"] != a || d["to"] != b || d["contact_from"] != ca || d["contact_to"] != cb {
		t.Fatalf("audit details = %#v", f.rec.entries[before].Details)
	}
	for _, p := range []int64{f.naProject, old} {
		got, err = f.svc.ChangeProject(ctx, f.actor, c.ID, p)
		if err != nil || got.BuilderContactID != nil {
			t.Fatalf("to %d: contact = %v, %v; want none", p, got.BuilderContactID, err)
		}
	}
}

func TestSetCaseBuilderContact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	a := addContact(t, f, p, "9000000101")
	b := addContact(t, f, p, "9000000102")
	c, _ := f.svc.Create(ctx, f.actor, caseOn(f, p, a))
	before := len(f.rec.entries)
	got, err := f.svc.SetBuilderContact(ctx, f.actor, c.ID, b)
	if err != nil || got.BuilderContactID == nil || *got.BuilderContactID != b {
		t.Fatalf("set = %v, %v", got.BuilderContactID, err)
	}
	e := f.rec.entries[before]
	d, _ := e.Details.(map[string]any)
	if e.Action != "case.builder_contact_changed" || d["from"] != a || d["to"] != b {
		t.Fatalf("audit = %+v", e)
	}
	removeContact(t, f, a)
	if _, err := f.svc.SetBuilderContact(ctx, f.actor, c.ID, a); !errors.Is(err, ErrBuilderContactInvalid) {
		t.Fatalf("removed err = %v, want ErrBuilderContactInvalid", err)
	}
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetBuilderContact(ctx, f.actor, c.ID, b); !errors.Is(err, ErrCaseDropped) {
		t.Fatalf("dropped err = %v, want ErrCaseDropped", err)
	}
}

func TestGetCaseShowsRemovedContact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Prestige")
	a := addContact(t, f, p, "9000000101")
	addContact(t, f, p, "9000000102")
	c, _ := f.svc.Create(ctx, f.actor, caseOn(f, p, a))
	removeContact(t, f, a)
	got, err := f.svc.Get(ctx, c.ID)
	if err != nil || got.BuilderContact == nil || !got.BuilderContact.Removed || got.BuilderContact.Name != "Ravi" || got.BuilderContact.Label != "Sales" {
		t.Fatalf("contact = %+v, %v", got.BuilderContact, err)
	}
}
