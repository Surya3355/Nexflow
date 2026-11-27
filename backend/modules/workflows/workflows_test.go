package workflows

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(pool, history.Nop{}), pool
}

// actor inserts a user and returns it as an access.User.
func actor(t *testing.T, pool *pgxpool.Pool, name string, role access.Role) access.User {
	t.Helper()
	var u access.User
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role)
		 VALUES ($1, $1, 'x', $2) RETURNING id`, name, string(role)).Scan(&id)
	if err != nil {
		t.Fatalf("seed actor: %v", err)
	}
	u.ID, u.Username, u.Role, u.IsActive = id, name, role, true
	return u
}

func codes(stages []Stage) []string {
	var out []string
	for _, s := range stages {
		out = append(out, s.Code)
	}
	return out
}

func TestBankWithoutOwnStagesUsesStandardStages(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	bank, err := svc.CreateBank(context.Background(), admin, "HDFC")
	if err != nil {
		t.Fatalf("CreateBank: %v", err)
	}
	stages, err := svc.StagesFor(context.Background(), bank.ID)
	if err != nil {
		t.Fatalf("StagesFor: %v", err)
	}
	want := []string{"received", "logged_in", "under_sanction", "sanctioned", "documentation", "disbursed"}
	if got := codes(stages); len(got) != len(want) || got[0] != want[0] || got[5] != want[5] {
		t.Fatalf("stages = %v, want standard %v", got, want)
	}
}

func TestBankWithOwnStagesUsesThemOnly(t *testing.T) {
	svc, pool := setup(t)
	ctx := context.Background()
	admin := actor(t, pool, "root", access.RoleAdmin)
	bank, _ := svc.CreateBank(ctx, admin, "SBI")
	if _, err := svc.AddStage(ctx, admin, &bank.ID, "sbi_docs", "SBI documents", 1, false); err != nil {
		t.Fatalf("AddStage: %v", err)
	}
	stages, err := svc.StagesFor(ctx, bank.ID)
	if err != nil {
		t.Fatalf("StagesFor: %v", err)
	}
	if got := codes(stages); len(got) != 1 || got[0] != "sbi_docs" {
		t.Fatalf("stages = %v, want only [sbi_docs]", got)
	}
}

func TestManagerCannotChangeStages(t *testing.T) {
	svc, pool := setup(t)
	ctx := context.Background()
	manager := actor(t, pool, "lead", access.RoleManager)
	if _, err := svc.AddStage(ctx, manager, nil, "x", "X", 9, false); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("manager AddStage err = %v, want ErrForbidden", err)
	}
	if err := svc.SetStageRequiresDocument(ctx, manager, 1, true); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("manager SetStageRequiresDocument err = %v, want ErrForbidden", err)
	}
}

func TestEmployeeCannotChangeStages(t *testing.T) {
	svc, pool := setup(t)
	ctx := context.Background()
	employee := actor(t, pool, "staff", access.RoleEmployee)
	if _, err := svc.AddStage(ctx, employee, nil, "x", "X", 9, false); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("employee AddStage err = %v, want ErrForbidden", err)
	}
	if _, err := svc.CreateBank(ctx, employee, "ICICI"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("employee CreateBank err = %v, want ErrForbidden", err)
	}
}

func TestSetRequiresDocumentOnStage(t *testing.T) {
	svc, pool := setup(t)
	ctx := context.Background()
	admin := actor(t, pool, "root", access.RoleAdmin)
	bank, _ := svc.CreateBank(ctx, admin, "HDFC")

	var sanctionedID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='sanctioned' AND bank_id IS NULL`).Scan(&sanctionedID); err != nil {
		t.Fatalf("find stage: %v", err)
	}
	if err := svc.SetStageRequiresDocument(ctx, admin, sanctionedID, true); err != nil {
		t.Fatalf("SetStageRequiresDocument: %v", err)
	}
	stages, err := svc.StagesFor(ctx, bank.ID)
	if err != nil {
		t.Fatalf("StagesFor: %v", err)
	}
	for _, s := range stages {
		if s.Code == "sanctioned" && !s.RequiresDocument {
			t.Fatal("sanctioned stage does not require documents after setting it")
		}
		if s.Code != "sanctioned" && s.RequiresDocument {
			t.Fatalf("stage %s unexpectedly requires documents", s.Code)
		}
	}
}

func TestDropReasonListIncludesOther(t *testing.T) {
	svc, _ := setup(t)
	reasons, err := svc.DropReasons(context.Background())
	if err != nil {
		t.Fatalf("DropReasons: %v", err)
	}
	for _, r := range reasons {
		if r.Value == "Other" {
			return
		}
	}
	t.Fatalf("drop reasons %v do not include Other", reasons)
}
