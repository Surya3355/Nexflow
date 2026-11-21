package access

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedCaseFor inserts a case assigned to user with the given status.
func seedCaseFor(t *testing.T, pool *pgxpool.Pool, user int64, mobile, status string) {
	t.Helper()
	ctx := context.Background()
	var customer, bank int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('C', $1) RETURNING id`, mobile).Scan(&customer); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('B' || $1) RETURNING id`, mobile).Scan(&bank); err != nil {
		t.Fatalf("bank: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, status)
		VALUES ($1, $2, 1000, (SELECT id FROM stages WHERE code = 'received' AND bank_id IS NULL), $3, $4)`,
		customer, bank, user, status); err != nil {
		t.Fatalf("case: %v", err)
	}
}

func TestListTeamCountsOpenCases(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	seedCaseFor(t, pool, staff.ID, "9000000001", "active")
	seedCaseFor(t, pool, staff.ID, "9000000002", "active")
	seedCaseFor(t, pool, staff.ID, "9000000003", "completed")

	team, err := svc.ListTeam(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListTeam: %v", err)
	}
	found := false
	for _, m := range team {
		if m.ID == staff.ID {
			found = true
			if m.OpenCases != 2 || m.Role != RoleEmployee {
				t.Fatalf("staff = %+v, want 2 open cases", m)
			}
		}
	}
	if !found || len(team) != 2 {
		t.Fatalf("team = %+v, want root and staff", team)
	}
}

func TestListTeamIncludesInactive(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	gone := seedActor(t, pool, "gone", RoleEmployee)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET is_active = false WHERE id = $1`, gone.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	team, err := svc.ListTeam(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListTeam: %v", err)
	}
	for _, m := range team {
		if m.ID == gone.ID && !m.IsActive {
			return
		}
	}
	t.Fatalf("inactive user missing or marked active: %+v", team)
}

func TestListTeamAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	manager := seedActor(t, pool, "lead", RoleManager)
	if _, err := svc.ListTeam(context.Background(), manager); !errors.Is(err, ErrForbidden) {
		t.Fatalf("manager err = %v, want ErrForbidden", err)
	}
}
