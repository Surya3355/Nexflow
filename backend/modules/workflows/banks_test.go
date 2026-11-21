package workflows

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestRenameBank(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, err := svc.CreateBank(ctx, admin, "HDFC")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.RenameBank(ctx, admin, b.ID, "  HDFC Bank  ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Name != "HDFC Bank" {
		t.Fatalf("name = %q, want trimmed 'HDFC Bank'", got.Name)
	}
	if _, err := svc.RenameBank(ctx, admin, b.ID, "   "); !errors.Is(err, ErrBankNameRequired) {
		t.Fatalf("empty name err = %v, want ErrBankNameRequired", err)
	}
}

func TestRenameBankDuplicate(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	if _, err := svc.CreateBank(ctx, admin, "SBI"); err != nil {
		t.Fatalf("create: %v", err)
	}
	b, err := svc.CreateBank(ctx, admin, "ICICI")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.RenameBank(ctx, admin, b.ID, "SBI"); !errors.Is(err, ErrBankNameTaken) {
		t.Fatalf("err = %v, want ErrBankNameTaken", err)
	}
}

func TestRenameBankAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	manager := actor(t, pool, "lead", access.RoleManager)
	b, err := svc.CreateBank(context.Background(), admin, "Axis")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.RenameBank(context.Background(), manager, b.ID, "Axis Bank"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestListBanksHasOwnStages(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	own, err := svc.CreateBank(ctx, admin, "Own")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	std, err := svc.CreateBank(ctx, admin, "Std")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO stages (bank_id, code, name, sort_order) VALUES ($1, 'received', 'Received', 1)`, own.ID); err != nil {
		t.Fatalf("stage: %v", err)
	}
	banks, err := svc.ListBanks(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, b := range banks {
		if b.ID == own.ID && !b.HasOwnStages {
			t.Fatal("Own should have own stages")
		}
		if b.ID == std.ID && b.HasOwnStages {
			t.Fatal("Std should use standard stages")
		}
	}
}
