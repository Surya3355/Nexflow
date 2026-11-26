package officers

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	svc      *Service
	pool     *pgxpool.Pool
	admin    access.User
	employee access.User
	hdfc     int64
	sbi      int64
}

func user(t *testing.T, pool *pgxpool.Pool, name string, role access.Role) access.User {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ($1, $1, 'x', $2) RETURNING id`, name, string(role)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return access.User{ID: id, Username: name, Role: role, IsActive: true}
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wf := workflows.NewService(pool, history.Nop{})
	f := fixture{svc: NewService(pool, wf, history.NewPgRecorder(pool)), pool: pool}
	f.admin = user(t, pool, "root", access.RoleAdmin)
	f.employee = user(t, pool, "emp", access.RoleEmployee)
	h, err := wf.CreateBank(ctx, f.admin, "HDFC")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.SetBankStageList(ctx, f.admin, h.ID, workflows.StageListHDFC); err != nil {
		t.Fatal(err)
	}
	s, err := wf.CreateBank(ctx, f.admin, "SBI")
	if err != nil {
		t.Fatal(err)
	}
	f.hdfc, f.sbi = h.ID, s.ID
	return f
}

func TestAddOfficer(t *testing.T) {
	f := setup(t)
	o, err := f.svc.Add(context.Background(), f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "  Ravi  ", Phone: "98765 43210"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if o.Name != "Ravi" || o.Phone != "9876543210" || o.TeamName != "DOVR" || o.BankName != "HDFC" {
		t.Fatalf("officer = %+v", o)
	}
	list, err := f.svc.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != o.ID {
		t.Fatalf("list = %+v %v", list, err)
	}
}

func TestAddOfficerChecks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cases := []struct {
		in   NewOfficer
		want error
	}{
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamOfficer, Name: "A", Phone: "9876543210"}, ErrTeamNotForBank},
		{NewOfficer{BankID: f.sbi, Team: workflows.TeamDOVR, Name: "A", Phone: "9876543210"}, ErrTeamNotForBank},
		{NewOfficer{BankID: f.hdfc, Team: "nope", Name: "A", Phone: "9876543210"}, ErrTeamNotForBank},
		{NewOfficer{BankID: 9999, Team: workflows.TeamDOVR, Name: "A", Phone: "9876543210"}, ErrTeamNotForBank},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "  ", Phone: "9876543210"}, ErrNameRequired},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: string(make([]byte, 81)), Phone: "9876543210"}, ErrNameRequired},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "A", Phone: "12345"}, ErrPhoneInvalid},
	}
	for _, c := range cases {
		if _, err := f.svc.Add(ctx, f.employee, c.in); !errors.Is(err, c.want) {
			t.Errorf("Add(%+v) err = %v, want %v", c.in, err, c.want)
		}
	}
	if _, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.sbi, Team: workflows.TeamOfficer, Name: "S", Phone: "9876543210"}); err != nil {
		t.Fatalf("SBI officer: %v", err)
	}
}

func TestAddOfficerPhoneRules(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Ravi", Phone: "9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Ravi K", Phone: "9876543210"}); !errors.Is(err, ErrPhoneTaken) {
		t.Fatalf("same team err = %v, want ErrPhoneTaken", err)
	}
	if _, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamAppraisal, Name: "Ravi", Phone: "9876543210"}); err != nil {
		t.Fatalf("same phone other team: %v", err)
	}
	if err := f.svc.Remove(ctx, f.employee, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Ravi", Phone: "9876543210"}); err != nil {
		t.Fatalf("re-add after remove: %v", err)
	}
}

func TestUpdateAndRemoveOfficer(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, _ := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Ravi", Phone: "9876543210"})
	b, _ := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Kumar", Phone: "9876543211"})
	u, err := f.svc.Update(ctx, f.employee, a.ID, "Ravi Teja", "9876500000", "")
	if err != nil || u.Name != "Ravi Teja" || u.Phone != "9876500000" {
		t.Fatalf("Update = %+v %v", u, err)
	}
	if _, err := f.svc.Update(ctx, f.employee, a.ID, "Ravi", "9876543211", ""); !errors.Is(err, ErrPhoneTaken) {
		t.Fatalf("update to taken phone err = %v", err)
	}
	if _, err := f.svc.Update(ctx, f.employee, a.ID, "", "9876543210", ""); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("blank name err = %v", err)
	}
	if err := f.svc.Remove(ctx, f.employee, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Remove(ctx, f.employee, b.ID); !errors.Is(err, ErrOfficerNotFound) {
		t.Fatalf("remove twice err = %v", err)
	}
	if _, err := f.svc.Update(ctx, f.employee, b.ID, "Kumar", "9876543211", ""); !errors.Is(err, ErrOfficerNotFound) {
		t.Fatalf("update removed err = %v", err)
	}
	list, _ := f.svc.List(ctx)
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("list = %+v", list)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('officer.added','officer.updated','officer.removed')`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("audit rows = %d %v, want 4", n, err)
	}
}

func TestListOrder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.sbi, Team: workflows.TeamOfficer, Name: "Zed", Phone: "9000000001"})
	f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamFixing, Name: "Amar", Phone: "9000000002"})
	f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamTDEP, Name: "Bala", Phone: "9000000003"})
	f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamTDEP, Name: "Anil", Phone: "9000000004"})
	list, _ := f.svc.List(ctx)
	var got []string
	for _, o := range list {
		got = append(got, o.Name)
	}
	want := []string{"Anil", "Bala", "Amar", "Zed"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// failingRecorder writes nothing and fails, like an audit insert that errors.
type failingRecorder struct{}

func (failingRecorder) Record(context.Context, history.Entry) error { return errors.New("audit down") }

func TestOfficerChangeRolledBackWhenAuditFails(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Ravi", Phone: "9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	broken := NewService(f.pool, workflows.NewService(f.pool, history.Nop{}), failingRecorder{})
	if _, err := broken.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Kumar", Phone: "9876543299"}); err == nil {
		t.Fatal("Add with failing audit should fail")
	}
	if _, err := broken.Update(ctx, f.employee, o.ID, "Ravi Changed", "9876543210", ""); err == nil {
		t.Fatal("Update with failing audit should fail")
	}
	if err := broken.Remove(ctx, f.employee, o.ID); err == nil {
		t.Fatal("Remove with failing audit should fail")
	}
	list, _ := f.svc.List(ctx)
	if len(list) != 1 || list[0].Name != "Ravi" {
		t.Fatalf("officers after failed changes = %+v, want only Ravi unchanged", list)
	}
}

func TestOfficerMobileOrEmail(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Mail Only", Email: "  ravi@hdfc.example "})
	if err != nil {
		t.Fatalf("email only: %v", err)
	}
	if o.Phone != "" || o.Email != "ravi@hdfc.example" {
		t.Fatalf("officer = %+v", o)
	}
	both, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Both", Phone: "9876543210", Email: "both@hdfc.example"})
	if err != nil || both.Phone != "9876543210" || both.Email != "both@hdfc.example" {
		t.Fatalf("both = %+v %v", both, err)
	}
	checks := []struct {
		in   NewOfficer
		want error
	}{
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "None"}, ErrContactRequired},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Bad", Email: "not-an-email"}, ErrEmailInvalid},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Bad", Email: "a b@x.example"}, ErrEmailInvalid},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Bad", Phone: "123", Email: "ok@x.example"}, ErrPhoneInvalid},
		{NewOfficer{BankID: f.hdfc, Team: workflows.TeamDOVR, Name: "Dup", Email: "RAVI@hdfc.example"}, ErrEmailTaken},
	}
	for _, c := range checks {
		if _, err := f.svc.Add(ctx, f.employee, c.in); !errors.Is(err, c.want) {
			t.Errorf("Add(%+v) err = %v, want %v", c.in, err, c.want)
		}
	}
	if _, err := f.svc.Add(ctx, f.employee, NewOfficer{BankID: f.hdfc, Team: workflows.TeamAppraisal, Name: "Other team", Email: "ravi@hdfc.example"}); err != nil {
		t.Fatalf("same email other team: %v", err)
	}
	u, err := f.svc.Update(ctx, f.employee, both.ID, "Both", "", "both@hdfc.example")
	if err != nil || u.Phone != "" {
		t.Fatalf("drop mobile, keep email = %+v %v", u, err)
	}
	if _, err := f.svc.Update(ctx, f.employee, both.ID, "Both", "", ""); !errors.Is(err, ErrContactRequired) {
		t.Fatalf("update to neither err = %v", err)
	}
}
