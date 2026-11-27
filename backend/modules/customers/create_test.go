package customers

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*Service, *pgxpool.Pool, access.User) {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role)
		 VALUES ('staff', 'Staff', 'x', 'employee') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	actor := access.User{ID: id, Username: "staff", Role: access.RoleEmployee, IsActive: true}
	return NewService(pool, history.Nop{}), pool, actor
}

func countCustomers(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM customers`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestCreateCustomerNewMobile(t *testing.T) {
	svc, _, actor := setup(t)
	res, err := svc.Create(context.Background(), actor, NewCustomer{Name: "Ravi", Mobile: "9876543210"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Existing != nil {
		t.Fatalf("Existing = %+v, want nil for a new mobile", res.Existing)
	}
	if res.Customer.Status != "active" || res.Customer.Mobile != "9876543210" {
		t.Fatalf("customer = %+v, want active with mobile 9876543210", res.Customer)
	}
}

func TestCreateCustomerExistingMobileWarnsAndDoesNotDuplicate(t *testing.T) {
	svc, pool, actor := setup(t)
	ctx := context.Background()
	first, err := svc.Create(ctx, actor, NewCustomer{Name: "Ravi", Mobile: "9876543210"})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	second, err := svc.Create(ctx, actor, NewCustomer{Name: "Ravi again", Mobile: "9876543210"})
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if second.Existing == nil || second.Existing.ID != first.Customer.ID {
		t.Fatalf("second.Existing = %+v, want customer %d", second.Existing, first.Customer.ID)
	}
	if n := countCustomers(t, pool); n != 1 {
		t.Fatalf("customer count = %d, want 1", n)
	}
}

func TestCreateCustomerPastedMobileIsNormalized(t *testing.T) {
	svc, pool, actor := setup(t)
	ctx := context.Background()
	first, _ := svc.Create(ctx, actor, NewCustomer{Name: "Ravi", Mobile: "9876543210"})
	res, err := svc.Create(ctx, actor, NewCustomer{Name: "Ravi", Mobile: "+91 98765 43210"})
	if err != nil {
		t.Fatalf("Create pasted: %v", err)
	}
	if res.Existing == nil || res.Existing.ID != first.Customer.ID {
		t.Fatalf("pasted mobile did not match existing customer: %+v", res)
	}
	if n := countCustomers(t, pool); n != 1 {
		t.Fatalf("customer count = %d, want 1", n)
	}
}

func TestConcurrentCreateSameMobileCreatesOneCustomer(t *testing.T) {
	svc, pool, actor := setup(t)
	ctx := context.Background()

	const workers = 5
	var wg sync.WaitGroup
	results := make([]CreateResult, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Create(ctx, actor, NewCustomer{Name: "Same", Mobile: "9000000001"})
		}(i)
	}
	wg.Wait()

	created := 0
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d returned error: %v", i, errs[i])
		}
		if results[i].Existing == nil {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("%d workers created a customer, want exactly 1", created)
	}
	if n := countCustomers(t, pool); n != 1 {
		t.Fatalf("customer count = %d, want 1", n)
	}
}

func TestCreateCustomerRejectsInvalidMobile(t *testing.T) {
	svc, pool, actor := setup(t)
	_, err := svc.Create(context.Background(), actor, NewCustomer{Name: "Bad", Mobile: "12345"})
	if !errors.Is(err, ErrInvalidMobile) {
		t.Fatalf("err = %v, want ErrInvalidMobile", err)
	}
	if n := countCustomers(t, pool); n != 0 {
		t.Fatalf("customer count = %d after invalid input, want 0", n)
	}
}
