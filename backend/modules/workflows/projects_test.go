package workflows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pin is a complete project input with one contact (phone optional).
func pin(name string, phone ...string) ProjectInput {
	ph := "9876543210"
	if len(phone) > 0 {
		ph = phone[0]
	}
	return ProjectInput{Name: name, Type: "villas", Address: "Road", Contacts: []ContactInput{{Name: "Ravi", Phone: ph}}}
}

// keep is a save of p under name that keeps all its active contacts.
func keep(p Project, name string) ProjectInput {
	in := ProjectInput{Name: name, Address: p.Address}
	if p.Type != nil {
		in.Type = *p.Type
	}
	for _, c := range p.Contacts {
		in.Contacts = append(in.Contacts, ContactInput{ID: c.ID, Name: c.Name, Phone: c.Phone, Email: c.Email, Label: c.Label})
	}
	return in
}

// setupAudited is setup with the real recorder, for tests that check audit rows.
func setupAudited(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	_, pool := setup(t)
	return NewService(pool, history.NewPgRecorder(pool)), pool
}

func auditCount(t *testing.T, pool *pgxpool.Pool, action string, id int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = $1 AND entity_id = $2`, action, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

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
		if _, err := svc.CreateProject(ctx, admin, pin(n, "90000000"+fmt.Sprint(10+len(n)))); err != nil {
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

	p, err := svc.CreateProject(ctx, admin, pin("  Prestige  "))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Name != "Prestige" {
		t.Fatalf("name = %q, want trimmed", p.Name)
	}
	if _, err := svc.CreateProject(ctx, admin, pin("prestige")); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("duplicate err = %v, want ErrProjectNameTaken", err)
	}
	if _, err := svc.CreateProject(ctx, admin, pin("   ")); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("empty err = %v, want ErrProjectNameRequired", err)
	}
	if _, err := svc.CreateProject(ctx, admin, pin("na")); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("NA duplicate err = %v, want ErrProjectNameTaken", err)
	}
}

func TestSaveProjectRenames(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()

	p, _ := svc.CreateProject(ctx, admin, pin("Prestige"))
	other, _ := svc.CreateProject(ctx, admin, pin("Sobha", "9000000099"))
	in := keep(p, "Prestige Lakeside")
	got, err := svc.SaveProject(ctx, admin, p.ID, in)
	if err != nil || got.Name != "Prestige Lakeside" {
		t.Fatalf("save = %+v, %v", got, err)
	}
	if _, err := svc.SaveProject(ctx, admin, p.ID, keep(got, "Prestige Lakeside")); err != nil {
		t.Fatalf("save own name: %v", err)
	}
	if _, err := svc.SaveProject(ctx, admin, other.ID, keep(other, "PRESTIGE LAKESIDE ")); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("duplicate rename err = %v, want ErrProjectNameTaken", err)
	}
	if _, err := svc.SaveProject(ctx, admin, 999999, pin("X")); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("unknown err = %v, want ErrProjectNotFound", err)
	}
	if _, err := svc.SaveProject(ctx, admin, p.ID, keep(got, "  ")); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("empty rename err = %v, want ErrProjectNameRequired", err)
	}
}

func TestProjectNALocked(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	ps, _ := svc.ListProjects(ctx)
	if _, err := svc.SaveProject(ctx, admin, ps[0].ID, pin("None")); !errors.Is(err, ErrProjectLocked) {
		t.Fatalf("save NA err = %v, want ErrProjectLocked", err)
	}
}

func TestProjectAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, pin("Prestige"))
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		u := actor(t, pool, "u-"+string(role), role)
		if _, err := svc.CreateProject(ctx, u, pin("X"+string(role))); !errors.Is(err, access.ErrForbidden) {
			t.Errorf("%s create err = %v, want forbidden", role, err)
		}
		if _, err := svc.SaveProject(ctx, u, p.ID, keep(p, "Y")); !errors.Is(err, access.ErrForbidden) {
			t.Errorf("%s save err = %v, want forbidden", role, err)
		}
	}
}

func TestCreateProjectNeedsDetails(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	cases := []struct {
		name string
		edit func(*ProjectInput)
		want error
	}{
		{"no type", func(in *ProjectInput) { in.Type = "" }, ErrProjectTypeRequired},
		{"flats", func(in *ProjectInput) { in.Type = "flats" }, ErrProjectTypeRequired},
		{"blank address", func(in *ProjectInput) { in.Address = "  " }, ErrProjectAddressRequired},
		{"no contacts", func(in *ProjectInput) { in.Contacts = nil }, ErrProjectContactRequired},
		{"bad phone", func(in *ProjectInput) { in.Contacts[0].Phone = "123" }, customers.ErrInvalidMobile},
		{"blank contact name", func(in *ProjectInput) { in.Contacts[0].Name = " " }, ErrContactNameRequired},
		{"long label", func(in *ProjectInput) { in.Contacts[0].Label = strings.Repeat("x", 41) }, ErrContactLabelTooLong},
		{"same phone twice", func(in *ProjectInput) {
			in.Contacts = append(in.Contacts, ContactInput{Name: "B", Phone: "+91 " + in.Contacts[0].Phone})
		}, ErrContactPhoneTaken},
	}
	for _, c := range cases {
		in := pin("P " + c.name)
		c.edit(&in)
		if _, err := svc.CreateProject(ctx, admin, in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestCreateProjectSavesContacts(t *testing.T) {
	svc, pool := setupAudited(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	in := ProjectInput{Name: "Green Meadows", Type: "villas", Address: " Survey 12, Kollur ", Contacts: []ContactInput{
		{Name: " Ravi K ", Phone: "+91 98765-43210", Email: " ravi@x.in ", Label: " Sales office "},
		{Name: "Site", Phone: "9000000002"},
	}}
	p, err := svc.CreateProject(ctx, admin, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type == nil || *got.Type != "villas" || got.Address != "Survey 12, Kollur" {
		t.Fatalf("details = %v %q", got.Type, got.Address)
	}
	if len(got.Contacts) != 2 || got.Contacts[0].ID > got.Contacts[1].ID {
		t.Fatalf("contacts = %+v", got.Contacts)
	}
	c := got.Contacts[0]
	if c.Name != "Ravi K" || c.Phone != "9876543210" || c.Email != "ravi@x.in" || c.Label != "Sales office" {
		t.Fatalf("contact = %+v", c)
	}
	if auditCount(t, pool, "project.created", p.ID) != 1 {
		t.Fatal("no project.created audit")
	}
}

func TestSaveProjectOldIncompleteMustComplete(t *testing.T) {
	svc, pool := setupAudited(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO projects (name) VALUES ('Old') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveProject(ctx, admin, id, ProjectInput{Name: "Old Two"}); !errors.Is(err, ErrProjectTypeRequired) {
		t.Fatalf("rename only err = %v, want ErrProjectTypeRequired", err)
	}
	if _, err := svc.SaveProject(ctx, admin, id, ProjectInput{Name: "Old Two", Type: "plots", Address: "A"}); !errors.Is(err, ErrProjectContactRequired) {
		t.Fatalf("no contact err = %v, want ErrProjectContactRequired", err)
	}
	if _, err := svc.SaveProject(ctx, admin, id, pin("Old Two")); err != nil {
		t.Fatalf("complete save: %v", err)
	}
	if auditCount(t, pool, "project.updated", id) != 1 {
		t.Fatal("no project.updated audit")
	}
}

func TestSaveProjectRemovesMissingContacts(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	in := pin("P")
	in.Contacts = append(in.Contacts, ContactInput{Name: "Two", Phone: "9000000002"})
	p, _ := svc.CreateProject(ctx, admin, in)
	p, _ = svc.GetProject(ctx, p.ID)
	first, second := p.Contacts[0], p.Contacts[1]
	save := keep(p, "P")
	save.Contacts = save.Contacts[1:]
	save.Contacts[0].Label = "Site office"
	if _, err := svc.SaveProject(ctx, admin, p.ID, save); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _ := svc.GetProject(ctx, p.ID)
	if len(got.Contacts) != 1 || got.Contacts[0].ID != second.ID || got.Contacts[0].Label != "Site office" {
		t.Fatalf("contacts = %+v", got.Contacts)
	}
	var removed bool
	if err := pool.QueryRow(ctx, `SELECT removed_at IS NOT NULL FROM project_contacts WHERE id = $1`, first.ID).Scan(&removed); err != nil || !removed {
		t.Fatalf("first contact removed = %v, %v", removed, err)
	}
	ps, _ := svc.ListProjects(ctx)
	for _, x := range ps {
		if x.ID == p.ID && x.ActiveContacts != 1 {
			t.Fatalf("active contacts = %d, want 1", x.ActiveContacts)
		}
	}
}

func TestSaveProjectLastContactRefused(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, pin("P"))
	save := keep(p, "Renamed")
	save.Contacts = nil
	if _, err := svc.SaveProject(ctx, admin, p.ID, save); !errors.Is(err, ErrProjectContactRequired) {
		t.Fatalf("err = %v, want ErrProjectContactRequired", err)
	}
	got, _ := svc.GetProject(ctx, p.ID)
	if got.Name != "P" || len(got.Contacts) != 1 {
		t.Fatalf("project changed: %+v", got)
	}
}

func TestSaveProjectRemoveAndReaddSamePhone(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, pin("P"))
	p, _ = svc.GetProject(ctx, p.ID)
	save := ProjectInput{Name: "P", Type: "villas", Address: "Road", Contacts: []ContactInput{
		{Name: "Ravi again", Phone: p.Contacts[0].Phone, Label: "Accounts"},
	}}
	if _, err := svc.SaveProject(ctx, admin, p.ID, save); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _ := svc.GetProject(ctx, p.ID)
	if len(got.Contacts) != 1 || got.Contacts[0].ID == p.Contacts[0].ID {
		t.Fatalf("contacts = %+v", got.Contacts)
	}
}

func TestSaveProjectForeignContactID(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	a, _ := svc.CreateProject(ctx, admin, pin("A"))
	b, _ := svc.CreateProject(ctx, admin, pin("B", "9000000077"))
	b, _ = svc.GetProject(ctx, b.ID)
	save := pin("A")
	save.Contacts = []ContactInput{{ID: b.Contacts[0].ID, Name: "X", Phone: "9000000011"}}
	if _, err := svc.SaveProject(ctx, admin, a.ID, save); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("err = %v, want ErrContactNotFound", err)
	}
}

func TestSaveProjectSamePhoneOtherProject(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	if _, err := svc.CreateProject(ctx, admin, pin("A")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProject(ctx, admin, pin("B")); err != nil {
		t.Fatalf("same phone on another project: %v", err)
	}
}

func TestSaveProjectConcurrentRemovals(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	in := pin("P")
	in.Contacts = append(in.Contacts, ContactInput{Name: "Y", Phone: "9000000002"})
	p, _ := svc.CreateProject(ctx, admin, in)
	p, _ = svc.GetProject(ctx, p.ID)
	onlyX, onlyY := keep(p, "P"), keep(p, "P")
	onlyX.Contacts, onlyY.Contacts = onlyX.Contacts[:1], onlyY.Contacts[1:]
	errs := make(chan error, 2)
	for _, s := range []ProjectInput{onlyX, onlyY} {
		go func(s ProjectInput) { _, err := svc.SaveProject(ctx, admin, p.ID, s); errs <- err }(s)
	}
	failed := 0
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			if !errors.Is(err, ErrContactNotFound) && !errors.Is(err, ErrProjectContactRequired) {
				t.Fatalf("unexpected err: %v", err)
			}
			failed++
		}
	}
	got, _ := svc.GetProject(ctx, p.ID)
	if len(got.Contacts) != 1 || failed != 1 {
		t.Fatalf("active = %d, failed saves = %d; want 1 and 1", len(got.Contacts), failed)
	}
}

func TestProjectCounts(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	p, _ := svc.CreateProject(ctx, admin, pin("Prestige"))

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
