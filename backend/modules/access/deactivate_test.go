package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeReassigner records the handover it was asked to do.
type fakeReassigner struct {
	from, to int64
	called   bool
}

func (f *fakeReassigner) ReassignAllTx(_ context.Context, from, to int64) (int, int, int, error) {
	f.called, f.from, f.to = true, from, to
	return 1, 2, 3, nil
}

// seedOpenCase gives user one active case, so they have open work.
func seedOpenCase(t *testing.T, pool *pgxpool.Pool, user int64) {
	t.Helper()
	seedCaseFor(t, pool, user, "9000000001", "active")
}

func isActive(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var on bool
	if err := pool.QueryRow(context.Background(), `SELECT is_active FROM users WHERE id = $1`, id).Scan(&on); err != nil {
		t.Fatalf("read user: %v", err)
	}
	return on
}

func TestOpenWorkCounts(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	seedOpenCase(t, pool, staff.ID)
	seedCaseFor(t, pool, staff.ID, "9000000002", "completed")
	w, err := svc.OpenWork(context.Background(), admin, staff.ID)
	if err != nil {
		t.Fatalf("OpenWork: %v", err)
	}
	if w.Cases != 1 || w.Leads != 0 || w.Queries != 0 {
		t.Fatalf("OpenWork = %+v, want 1 case only", w)
	}
}

func TestDeactivateMovesWork(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	other := seedActor(t, pool, "other", RoleEmployee)
	seedOpenCase(t, pool, staff.ID)
	fr := &fakeReassigner{}
	svc.SetReassigner(fr)

	to := other.ID
	if err := svc.Deactivate(context.Background(), admin, staff.ID, &to); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if !fr.called || fr.from != staff.ID || fr.to != other.ID {
		t.Fatalf("reassigner = %+v, want %d -> %d", fr, staff.ID, other.ID)
	}
	if isActive(t, pool, staff.ID) {
		t.Fatal("user still active")
	}
	// Their session ends: the per-request user load refuses them.
	if _, err := svc.loadActive(context.Background(), staff.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("loadActive after switch off: %v, want ErrUserNotFound", err)
	}
}

func TestDeactivateWithNothingOpen(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	fr := &fakeReassigner{}
	svc.SetReassigner(fr)
	if err := svc.Deactivate(context.Background(), admin, staff.ID, nil); err != nil {
		t.Fatalf("Deactivate with nothing open: %v", err)
	}
	if fr.called {
		t.Fatal("handover ran with nothing to move")
	}
}

func TestDeactivateNeedsTakeover(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	seedOpenCase(t, pool, staff.ID)
	svc.SetReassigner(&fakeReassigner{})
	if err := svc.Deactivate(context.Background(), admin, staff.ID, nil); !errors.Is(err, ErrTakeoverRequired) {
		t.Fatalf("err = %v, want ErrTakeoverRequired", err)
	}
	if !isActive(t, pool, staff.ID) {
		t.Fatal("user switched off despite the error")
	}
}

func TestTakeoverMustBeActiveOther(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	gone := seedActor(t, pool, "gone", RoleEmployee)
	seedOpenCase(t, pool, staff.ID)
	svc.SetReassigner(&fakeReassigner{})
	if _, err := pool.Exec(context.Background(), `UPDATE users SET is_active = false WHERE id = $1`, gone.ID); err != nil {
		t.Fatal(err)
	}
	for name, to := range map[string]int64{"inactive": gone.ID, "same person": staff.ID, "unknown": 999999} {
		id := to
		if err := svc.Deactivate(context.Background(), admin, staff.ID, &id); !errors.Is(err, ErrTakeoverInvalid) {
			t.Errorf("%s takeover: err = %v, want ErrTakeoverInvalid", name, err)
		}
	}
}

func TestCannotDeactivateSelf(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	seedActor(t, pool, "root2", RoleAdmin)
	if err := svc.Deactivate(context.Background(), admin, admin.ID, nil); !errors.Is(err, ErrSelfDeactivate) {
		t.Fatalf("err = %v, want ErrSelfDeactivate", err)
	}
}

func TestLastAdminProtected(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	second := seedActor(t, pool, "root2", RoleAdmin)
	ctx := context.Background()

	// With two admins, one may be demoted.
	if err := svc.ChangeRole(ctx, admin, second.ID, RoleManager); err != nil {
		t.Fatalf("demote second admin: %v", err)
	}
	// Now admin is the only one: demoting them is refused.
	if err := svc.ChangeRole(ctx, admin, admin.ID, RoleManager); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: err = %v, want ErrLastAdmin", err)
	}
	// Promote second back, switch admin off by second, then second is last.
	if err := svc.ChangeRole(ctx, admin, second.ID, RoleAdmin); err != nil {
		t.Fatalf("promote: %v", err)
	}
	second.Role = RoleAdmin
	if err := svc.Deactivate(ctx, second, admin.ID, nil); err != nil {
		t.Fatalf("switch off one of two admins: %v", err)
	}
	third := seedActor(t, pool, "root3", RoleAdmin)
	if _, err := pool.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, third.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeRole(ctx, second, second.ID, RoleEmployee); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last active admin: err = %v, want ErrLastAdmin", err)
	}
}

func TestNonAdminForbidden(t *testing.T) {
	svc, pool := setup(t)
	manager := seedActor(t, pool, "lead", RoleManager)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	ctx := context.Background()
	checks := map[string]error{
		"open work":  func() error { _, err := svc.OpenWork(ctx, manager, staff.ID); return err }(),
		"deactivate": svc.Deactivate(ctx, manager, staff.ID, nil),
		"reactivate": svc.Reactivate(ctx, manager, staff.ID),
		"role":       svc.ChangeRole(ctx, manager, staff.ID, RoleManager),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("%s by manager: err = %v, want ErrForbidden", name, err)
		}
	}
}

func TestReactivate(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	ctx := context.Background()
	if err := svc.Deactivate(ctx, admin, staff.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reactivate(ctx, admin, staff.ID); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if !isActive(t, pool, staff.ID) {
		t.Fatal("user not active again")
	}
	if _, err := svc.Authenticate(ctx, "staff", goodPassword); err != nil {
		t.Fatalf("login after switch on: %v", err)
	}
}

func TestChangeRoleInvalid(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	if err := svc.ChangeRole(context.Background(), admin, staff.ID, Role("boss")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("err = %v, want ErrInvalidRole", err)
	}
	if err := svc.ChangeRole(context.Background(), admin, 999999, RoleManager); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: err = %v, want ErrUserNotFound", err)
	}
}

func TestUserChangeAuditEntries(t *testing.T) {
	pool := setupPool(t)
	rec := &memRecorder{}
	svc := NewService(pool, rec)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	other := seedActor(t, pool, "other", RoleEmployee)
	seedOpenCase(t, pool, staff.ID)
	svc.SetReassigner(&fakeReassigner{})
	ctx := context.Background()

	if err := svc.ChangeRole(ctx, admin, staff.ID, RoleManager); err != nil {
		t.Fatal(err)
	}
	to := other.ID
	if err := svc.Deactivate(ctx, admin, staff.ID, &to); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reactivate(ctx, admin, staff.ID); err != nil {
		t.Fatal(err)
	}
	want := []string{"user.role_changed", "user.deactivated", "user.reactivated"}
	if len(rec.entries) != len(want) {
		t.Fatalf("entries = %+v, want %v", rec.entries, want)
	}
	for i, a := range want {
		if rec.entries[i].Action != a {
			t.Fatalf("entry %d = %s, want %s", i, rec.entries[i].Action, a)
		}
	}
	d, _ := rec.entries[0].Details.(map[string]any)
	if d["before"] != "employee" || d["after"] != "manager" {
		t.Fatalf("role change details = %v", rec.entries[0].Details)
	}
	d, _ = rec.entries[1].Details.(map[string]any)
	if d["takeover_user_id"] != other.ID || d["cases"] != 2 {
		t.Fatalf("deactivate details = %v", rec.entries[1].Details)
	}
}

// memRecorder keeps audit entries in memory.
type memRecorder struct{ entries []history.Entry }

func (m *memRecorder) Record(_ context.Context, e history.Entry) error {
	m.entries = append(m.entries, e)
	return nil
}

// setupPool returns a fresh migrated database.
func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_, pool := setup(t)
	return pool
}
