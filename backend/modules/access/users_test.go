package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

const goodPassword = "a-long-enough-password"

// setup returns a fresh migrated database and a Service on it.
func setup(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(pool, history.Nop{}), pool
}

// seedActor inserts a user directly (bootstrap) and returns it as an actor.
func seedActor(t *testing.T, pool *pgxpool.Pool, username string, role Role) User {
	t.Helper()
	hash, err := HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	var u User
	err = pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role)
		 VALUES ($1, $1, $2, $3) RETURNING id, username, full_name, role, is_active`,
		username, hash, string(role)).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.IsActive)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func TestCreateUserRequiresAdmin(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	manager := seedActor(t, pool, "lead", RoleManager)
	employee := seedActor(t, pool, "staff", RoleEmployee)
	ctx := context.Background()

	in := NewUser{Username: "newbie", FullName: "New Person", Role: RoleEmployee, Password: goodPassword}

	if _, err := svc.CreateUser(ctx, manager, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("manager CreateUser err = %v, want ErrForbidden", err)
	}
	in.Username = "newbie2"
	if _, err := svc.CreateUser(ctx, employee, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("employee CreateUser err = %v, want ErrForbidden", err)
	}
	in.Username = "newbie3"
	u, err := svc.CreateUser(ctx, admin, in)
	if err != nil {
		t.Fatalf("admin CreateUser: %v", err)
	}
	if u.Username != "newbie3" || u.Role != RoleEmployee || !u.IsActive {
		t.Fatalf("created user = %+v", u)
	}
}

func TestUsernameIsLowercasedAndUnique(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	ctx := context.Background()

	u, err := svc.CreateUser(ctx, admin, NewUser{Username: "Surya", FullName: "Surya", Role: RoleEmployee, Password: goodPassword})
	if err != nil {
		t.Fatalf("create Surya: %v", err)
	}
	if u.Username != "surya" {
		t.Fatalf("stored username = %q, want %q", u.Username, "surya")
	}
	_, err = svc.CreateUser(ctx, admin, NewUser{Username: "SURYA", FullName: "Other", Role: RoleEmployee, Password: goodPassword})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("second SURYA err = %v, want ErrUsernameTaken", err)
	}
}

func TestRoleMustBeKnown(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	_, err := svc.CreateUser(context.Background(), admin,
		NewUser{Username: "boss", FullName: "Boss", Role: Role("owner"), Password: goodPassword})
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("role owner err = %v, want ErrInvalidRole", err)
	}
}

func TestShortPasswordRefused(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	_, err := svc.CreateUser(context.Background(), admin,
		NewUser{Username: "short", FullName: "Short", Role: RoleEmployee, Password: "x"})
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password err = %v, want ErrWeakPassword", err)
	}
}

func TestCreateUserStoresEmailForAlerts(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	u, err := svc.CreateUser(context.Background(), admin, NewUser{Username: "asha", FullName: "Asha", Role: RoleEmployee, Password: goodPassword, Email: " asha@example.com "})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	var email string
	if err := pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id=$1`, u.ID).Scan(&email); err != nil {
		t.Fatalf("read email: %v", err)
	}
	if email != "asha@example.com" {
		t.Fatalf("stored email = %q, want trimmed asha@example.com", email)
	}
}
