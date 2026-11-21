package workflows

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func projectNames(ps []Project) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestProjectsNAFirst(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()

	ps, err := svc.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ps) != 1 || ps[0].Name != "NA" || !ps[0].IsSystem {
		t.Fatalf("fresh projects = %+v, want only system NA", ps)
	}
	for _, n := range []string{"Zenith", "Akshaya"} {
		if _, err := svc.CreateProject(ctx, admin, n); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	ps, _ = svc.ListProjects(ctx)
	got := projectNames(ps)
	want := []string{"NA", "Akshaya", "Zenith"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestCreateProjectTrimsAndRefusesDuplicate(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, admin, "  Prestige  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Name != "Prestige" {
		t.Fatalf("name = %q, want trimmed", p.Name)
	}
	if _, err := svc.CreateProject(ctx, admin, "prestige"); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("duplicate err = %v, want ErrProjectNameTaken", err)
	}
	if _, err := svc.CreateProject(ctx, admin, "   "); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("empty err = %v, want ErrProjectNameRequired", err)
	}
	if _, err := svc.CreateProject(ctx, admin, "na"); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("NA duplicate err = %v, want ErrProjectNameTaken", err)
	}
}

func TestRenameProject(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()

	p, _ := svc.CreateProject(ctx, admin, "Prestige")
	other, _ := svc.CreateProject(ctx, admin, "Sobha")
	got, err := svc.RenameProject(ctx, admin, p.ID, "Prestige Lakeside")
	if err != nil || got.Name != "Prestige Lakeside" {
		t.Fatalf("rename = %+v, %v", got, err)
	}
	if _, err := svc.RenameProject(ctx, admin, p.ID, "Prestige Lakeside"); err != nil {
		t.Fatalf("rename to own name: %v", err)
	}
	if _, err := svc.RenameProject(ctx, admin, other.ID, "PRESTIGE LAKESIDE "); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("duplicate rename err = %v, want ErrProjectNameTaken", err)
	}
	ps, _ := svc.ListProjects(ctx)
	if _, err := svc.RenameProject(ctx, admin, ps[0].ID, "None"); !errors.Is(err, ErrProjectLocked) {
		t.Fatalf("rename NA err = %v, want ErrProjectLocked", err)
	}
	if _, err := svc.RenameProject(ctx, admin, 999999, "X"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("unknown err = %v, want ErrProjectNotFound", err)
	}
	if _, err := svc.RenameProject(ctx, admin, p.ID, "  "); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("empty rename err = %v, want ErrProjectNameRequired", err)
	}
}

func TestProjectAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, "Prestige")
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		u := actor(t, pool, "u-"+string(role), role)
		if _, err := svc.CreateProject(ctx, u, "X"+string(role)); !errors.Is(err, access.ErrForbidden) {
			t.Errorf("%s create err = %v, want forbidden", role, err)
		}
		if _, err := svc.RenameProject(ctx, u, p.ID, "Y"); !errors.Is(err, access.ErrForbidden) {
			t.Errorf("%s rename err = %v, want forbidden", role, err)
		}
	}
}

func TestProjectCounts(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, "Prestige")

	var bank, customer int64
	if err := pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('A', '9000000001') RETURNING id`).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	reason := `(SELECT id FROM dropdown_values WHERE category = 'drop_reason' LIMIT 1)`
	for _, status := range []string{"active", "dropped"} {
		_, err := pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, project_id, loan_amount, current_stage_id, assignee_id, status, dropped_reason_id, dropped_at)
			VALUES ($1, $2, $3, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $4, $5,
			        CASE WHEN $5 = 'dropped' THEN `+reason+` END, CASE WHEN $5 = 'dropped' THEN now() END)`,
			customer, bank, p.ID, admin.ID, status)
		if err != nil {
			t.Fatalf("insert %s case: %v", status, err)
		}
	}
	ps, _ := svc.ListProjects(ctx)
	for _, got := range ps {
		if got.ID == p.ID {
			if got.OpenCases != 1 || got.TotalCases != 2 {
				t.Fatalf("counts = open %d total %d, want 1 and 2", got.OpenCases, got.TotalCases)
			}
			return
		}
	}
	t.Fatal("project not listed")
}
