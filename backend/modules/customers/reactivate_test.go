package customers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/history"
)

func TestReactivateClearsDrop(t *testing.T) {
	_, pool, actor := setup(t)
	svc := NewService(pool, history.NewPgRecorder(pool))
	ctx := context.Background()
	res, _ := svc.Create(ctx, actor, NewCustomer{Name: "Asha", Mobile: "9000000021"})
	id := res.Customer.ID
	oldCase := addCase(t, pool, actor.ID, id, "active")
	reason := reasonID(t, pool, "Not eligible")
	if _, err := svc.Drop(ctx, actor, id, DropInput{ReasonID: reason}); err != nil {
		t.Fatalf("drop: %v", err)
	}

	got, err := svc.Reactivate(ctx, actor, id)
	if err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}
	var nulls bool
	if err := pool.QueryRow(ctx, `SELECT dropped_reason_id IS NULL AND dropped_remark IS NULL AND dropped_at IS NULL AND dropped_by IS NULL
		FROM customers WHERE id = $1`, id).Scan(&nulls); err != nil || !nulls {
		t.Fatalf("drop fields cleared = %v (%v), want true", nulls, err)
	}
	var caseStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM cases WHERE id = $1`, oldCase).Scan(&caseStatus); err != nil || caseStatus != "dropped" {
		t.Fatalf("old case status = %q (%v), want dropped", caseStatus, err)
	}
	var details []byte
	if err := pool.QueryRow(ctx, `SELECT details FROM audit_log WHERE action = 'customer.reactivated' AND entity_id = $1`, id).Scan(&details); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	var d map[string]any
	_ = json.Unmarshal(details, &d)
	if d["dropped_reason_id"] != float64(reason) {
		t.Fatalf("audit details = %s, want previous reason %d", details, reason)
	}
}

func TestReactivateActiveCustomer(t *testing.T) {
	svc, _, actor := setup(t)
	ctx := context.Background()
	res, _ := svc.Create(ctx, actor, NewCustomer{Name: "Asha", Mobile: "9000000022"})
	if _, err := svc.Reactivate(ctx, actor, res.Customer.ID); !errors.Is(err, ErrNotDropped) {
		t.Fatalf("err = %v, want ErrNotDropped", err)
	}
	if _, err := svc.Reactivate(ctx, actor, 999999); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("unknown err = %v, want ErrCustomerNotFound", err)
	}
}
