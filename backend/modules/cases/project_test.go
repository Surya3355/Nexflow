package cases

import (
	"context"
	"errors"
	"testing"
)

// addProject inserts a project and returns its id.
func addProject(t *testing.T, f fixture, name string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO projects (name) VALUES ($1) RETURNING id`, name), &id)
	return id
}

func TestCreateCaseRequiresProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := NewCase{CustomerID: f.customer, BankID: f.bankID, LoanAmount: "500000", AssigneeID: f.actor.ID}

	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrProjectRequired) {
		t.Fatalf("no project err = %v, want ErrProjectRequired", err)
	}
	in.ProjectID = 999999
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("unknown project err = %v, want ErrProjectNotFound", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cases`); n != 0 {
		t.Fatalf("cases = %d after refusals, want 0", n)
	}
	prestige := addProject(t, f, "Prestige")
	in.ProjectID = prestige
	c, err := f.svc.Create(ctx, f.actor, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ProjectID != prestige {
		t.Fatalf("ProjectID = %d, want %d", c.ProjectID, prestige)
	}
	got, err := f.svc.Get(ctx, c.ID)
	if err != nil || got.ProjectID != prestige {
		t.Fatalf("Get ProjectID = %d (%v), want %d", got.ProjectID, err, prestige)
	}
}

func TestChangeProjectAudited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	prestige := addProject(t, f, "Prestige")
	before := len(f.rec.entries)

	got, err := f.svc.ChangeProject(ctx, f.actor, c.ID, prestige)
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	if got.ProjectID != prestige {
		t.Fatalf("ProjectID = %d, want %d", got.ProjectID, prestige)
	}
	entries := f.rec.entries[before:]
	if len(entries) != 1 || entries[0].Action != "case.project_changed" {
		t.Fatalf("audit = %+v, want one case.project_changed", entries)
	}
	d, ok := entries[0].Details.(map[string]int64)
	if !ok || d["from"] != f.naProject || d["to"] != prestige {
		t.Fatalf("details = %#v, want from %d to %d", entries[0].Details, f.naProject, prestige)
	}

	if _, err := f.svc.ChangeProject(ctx, f.actor, c.ID, prestige); err != nil {
		t.Fatalf("same project: %v", err)
	}
	if n := len(f.rec.entries) - before; n != 1 {
		t.Fatalf("audit entries = %d after no-op change, want 1", n)
	}
	if _, err := f.svc.ChangeProject(ctx, f.actor, c.ID, 999999); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("unknown project err = %v, want ErrProjectNotFound", err)
	}
	if _, err := f.svc.ChangeProject(ctx, f.actor, 999999, prestige); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("unknown case err = %v, want ErrCaseNotFound", err)
	}
}

func TestChangeProjectDroppedCase(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := f.svc.ChangeProject(ctx, f.actor, c.ID, addProject(t, f, "Prestige")); !errors.Is(err, ErrCaseDropped) {
		t.Fatalf("err = %v, want ErrCaseDropped", err)
	}
}

func TestListRowsHasProjectName(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	prestige := addProject(t, f, "Prestige")
	if _, err := f.svc.Create(ctx, f.actor, NewCase{CustomerID: f.customer, BankID: f.bankID, ProjectID: prestige, LoanAmount: "500000", AssigneeID: f.actor.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	if rows[0].ProjectName != "Prestige" || rows[0].ProjectID != prestige {
		t.Fatalf("row project = %d %q, want Prestige", rows[0].ProjectID, rows[0].ProjectName)
	}
}
