package customers

import (
	"context"
	"errors"
	"testing"
)

func TestAllStaffCanEditCustomer(t *testing.T) {
	svc, _, actor := setup(t)
	ctx := context.Background()
	res, err := svc.Create(ctx, actor, NewCustomer{Name: "Ravi", Mobile: "9876543210"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.Update(ctx, actor, res.Customer.ID, CustomerFields{Name: "Ravi Kumar", Mobile: "+91 98765 43210", Email: "ravi@example.com"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Ravi Kumar" || got.Email != "ravi@example.com" || got.Mobile != "9876543210" {
		t.Fatalf("updated customer = %+v", got)
	}
}

func TestEditMobileAlreadyUsedIsRefused(t *testing.T) {
	svc, _, actor := setup(t)
	ctx := context.Background()
	a, _ := svc.Create(ctx, actor, NewCustomer{Name: "A", Mobile: "9000000001"})
	if _, err := svc.Create(ctx, actor, NewCustomer{Name: "B", Mobile: "9000000002"}); err != nil {
		t.Fatalf("second customer: %v", err)
	}
	_, err := svc.Update(ctx, actor, a.Customer.ID, CustomerFields{Name: "A", Mobile: "9000000002"})
	if !errors.Is(err, ErrMobileInUse) {
		t.Fatalf("err = %v, want ErrMobileInUse", err)
	}
}
