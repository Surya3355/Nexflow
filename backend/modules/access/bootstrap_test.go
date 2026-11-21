package access

import (
	"context"
	"errors"
	"testing"
)

func TestCreateFirstAdminOnlyOnEmptyDatabase(t *testing.T) {
	_, pool := setup(t)
	ctx := context.Background()
	u, err := CreateFirstAdmin(ctx, pool, "owner", "Owner", goodPassword)
	if err != nil {
		t.Fatalf("first admin: %v", err)
	}
	if u.Role != RoleAdmin || u.Username != "owner" {
		t.Fatalf("created = %+v", u)
	}
	if _, err := CreateFirstAdmin(ctx, pool, "second", "Second", goodPassword); !errors.Is(err, ErrUsersExist) {
		t.Fatalf("second bootstrap err = %v, want ErrUsersExist", err)
	}
}
