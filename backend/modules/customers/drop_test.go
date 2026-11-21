package customers

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func reasonID(t *testing.T, pool *pgxpool.Pool, value string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM dropdown_values WHERE category='drop_reason' AND value=$1`, value).Scan(&id); err != nil {
		t.Fatalf("reason %q: %v", value, err)
	}
	return id
}

// addCase inserts a case for a customer directly, at the Received stage.
func addCase(t *testing.T, pool *pgxpool.Pool, actorID, customerID int64, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var bankID, stageID, id int64
	if err := pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`).Scan(&bankID); err != nil {
		t.Fatalf("bank: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`).Scan(&stageID); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, status)
		VALUES ($1, $2, 100000, $3, $4, $5) RETURNING id`, customerID, bankID, stageID, actorID, status).Scan(&id); err != nil {
		t.Fatalf("case: %v", err)
	}
	return id
}

func TestDropCustomerRequiresReason(t *testing.T) {
	svc, _, actor := setup(t)
	res, _ := svc.Create(context.Background(), actor, NewCustomer{Name: "A", Mobile: "9000000011"})
	_, err := svc.Drop(context.Background(), actor, res.Customer.ID, DropInput{})
	if !errors.Is(err, ErrInvalidReason) {
		t.Fatalf("err = %v, want ErrInvalidReason", err)
	}
}

func TestDroppingCustomerDropsAllItsCases(t *testing.T) {
	svc, pool, actor := setup(t)
	ctx := context.Background()
	res, _ := svc.Create(ctx, actor, NewCustomer{Name: "B", Mobile: "9000000012"})
	active := addCase(t, pool, actor.ID, res.Customer.ID, "active")
	completed := addCase(t, pool, actor.ID, res.Customer.ID, "completed")

	reason := reasonID(t, pool, "Customer not interested")
	if _, err := svc.Drop(ctx, actor, res.Customer.ID, DropInput{ReasonID: reason}); err != nil {
		t.Fatalf("Drop customer: %v", err)
	}
	for _, id := range []int64{active, completed} {
		var status string
		var viaCustomer bool
		var got int64
		if err := pool.QueryRow(ctx, `SELECT status, dropped_via_customer, dropped_reason_id FROM cases WHERE id=$1`, id).Scan(&status, &viaCustomer, &got); err != nil {
			t.Fatalf("read case %d: %v", id, err)
		}
		if status != "dropped" || !viaCustomer || got != reason {
			t.Fatalf("case %d: status=%s via_customer=%v reason=%d", id, status, viaCustomer, got)
		}
	}
}

// Owner answer A (round 13): an already-dropped case keeps its own reason.
func TestDroppingCustomerLeavesAlreadyDroppedCaseUnchanged(t *testing.T) {
	svc, pool, actor := setup(t)
	ctx := context.Background()
	res, _ := svc.Create(ctx, actor, NewCustomer{Name: "C", Mobile: "9000000013"})
	earlier := addCase(t, pool, actor.ID, res.Customer.ID, "active")
	notEligible := reasonID(t, pool, "Not eligible")
	if _, err := pool.Exec(ctx, `UPDATE cases SET status='dropped', dropped_reason_id=$1, dropped_at=now(), dropped_by=$2 WHERE id=$3`,
		notEligible, actor.ID, earlier); err != nil {
		t.Fatalf("seed dropped case: %v", err)
	}

	if _, err := svc.Drop(ctx, actor, res.Customer.ID, DropInput{ReasonID: reasonID(t, pool, "Customer unreachable")}); err != nil {
		t.Fatalf("Drop customer: %v", err)
	}
	var got int64
	var viaCustomer bool
	if err := pool.QueryRow(ctx, `SELECT dropped_reason_id, dropped_via_customer FROM cases WHERE id=$1`, earlier).Scan(&got, &viaCustomer); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != notEligible || viaCustomer {
		t.Fatalf("already-dropped case changed: reason=%d via_customer=%v", got, viaCustomer)
	}
}
