package cases

import (
	"context"
	"errors"
	"testing"
)

// typedProject adds a project with a type (a builder project).
func typedProject(t *testing.T, f fixture, name, typ string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO projects (name, type, address) VALUES ($1, $2, 'Road') RETURNING id`, name, typ), &id)
	return id
}

func unitCase(f fixture, project int64, unit string) NewCase {
	return NewCase{CustomerID: f.customer, BankID: f.bankID, ProjectID: project, LoanAmount: "100000", AssigneeID: f.actor.ID, UnitNumber: unit}
}

func TestUnitRequiredOnTypedProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := typedProject(t, f, "Villas", "villas")
	for _, u := range []string{"", "   "} {
		if _, err := f.svc.Create(ctx, f.actor, unitCase(f, p, u)); !errors.Is(err, ErrUnitRequired) {
			t.Errorf("unit %q err = %v, want ErrUnitRequired", u, err)
		}
	}
	c, err := f.svc.Create(ctx, f.actor, unitCase(f, p, "a-1012"))
	if err != nil || c.UnitNumber == nil || *c.UnitNumber != "A-1012" {
		t.Fatalf("create = %v, %v", c.UnitNumber, err)
	}
}

func TestUnitOptionalOnNAAndUntyped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, p := range []int64{f.naProject, addProject(t, f, "Old")} {
		if _, err := f.svc.Create(ctx, f.actor, unitCase(f, p, "")); err != nil {
			t.Errorf("project %d without unit: %v", p, err)
		}
	}
}

func TestFromLeadUnitRequired(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := addProject(t, f, "Old")
	var lead int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, $2, $3) RETURNING id`, f.customer, p, f.actor.ID), &lead)
	if _, err := f.pool.Exec(ctx, `UPDATE projects SET type = 'villas', address = 'Road' WHERE id = $1`, p); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateFromLead(ctx, f.actor, lead, FromLead{BankID: f.bankID, LoanAmount: "500000", AssigneeID: f.actor.ID}); !errors.Is(err, ErrUnitRequired) {
		t.Fatalf("err = %v, want ErrUnitRequired", err)
	}
}

func TestSetUnitCannotClearOnTyped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := typedProject(t, f, "Plots", "plots")
	c, err := f.svc.Create(ctx, f.actor, unitCase(f, p, "P-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetUnitNumber(ctx, f.actor, c.ID, " "); !errors.Is(err, ErrUnitRequired) {
		t.Fatalf("clear err = %v, want ErrUnitRequired", err)
	}
	if got, err := f.svc.SetUnitNumber(ctx, f.actor, c.ID, "p-2"); err != nil || *got.UnitNumber != "P-2" {
		t.Fatalf("change = %v, %v", got.UnitNumber, err)
	}
	na, _ := f.svc.Create(ctx, f.actor, unitCase(f, f.naProject, "X1"))
	if got, err := f.svc.SetUnitNumber(ctx, f.actor, na.ID, ""); err != nil || got.UnitNumber != nil {
		t.Fatalf("clear on NA = %v, %v", got.UnitNumber, err)
	}
}

func TestChangeProjectKeepsEmptyUnit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, f.actor, unitCase(f, f.naProject, ""))
	p := typedProject(t, f, "Tower", "high_rise")
	got, err := f.svc.ChangeProject(ctx, f.actor, c.ID, p)
	if err != nil || got.UnitNumber != nil {
		t.Fatalf("move = %v, %v", got.UnitNumber, err)
	}
	if got.ProjectType == nil || *got.ProjectType != "high_rise" {
		t.Fatalf("project type = %v", got.ProjectType)
	}
}

func TestGetCaseHasProjectType(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, f.actor, unitCase(f, f.naProject, ""))
	got, err := f.svc.Get(ctx, c.ID)
	if err != nil || got.ProjectType != nil {
		t.Fatalf("NA project type = %v, %v", got.ProjectType, err)
	}
}
