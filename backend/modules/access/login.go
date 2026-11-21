package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// dummyHash is verified against when the username is unknown, so an unknown
// user costs about as much as a wrong password. Computed once.
var (
	dummyOnce sync.Once
	dummyHash string
)

func getDummyHash() string {
	dummyOnce.Do(func() {
		dummyHash, _ = HashPassword("nexflow-dummy-password")
	})
	return dummyHash
}

// Authenticate checks a username and password and returns the active user.
// Unknown users, wrong passwords and inactive users all get ErrInvalidCredentials.
func (s *Service) Authenticate(ctx context.Context, username, password string) (User, error) {
	row, err := s.q.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(getDummyHash(), password)
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("access: lookup user: %w", err)
	}
	ok, err := VerifyPassword(row.PasswordHash, password)
	if err != nil {
		return User{}, fmt.Errorf("access: verify: %w", err)
	}
	if !ok || !row.IsActive {
		return User{}, ErrInvalidCredentials
	}
	return User{
		ID:                row.ID,
		Username:          row.Username,
		FullName:          row.FullName,
		Role:              Role(row.Role),
		IsActive:          row.IsActive,
		PasswordChangedAt: timePtr(row.PasswordChangedAt),
	}, nil
}

// loadActive reads the user by ID on every request. A user who was
// deactivated after login fails here, even with a valid session.
func (s *Service) loadActive(ctx context.Context, id int64) (User, error) {
	row, err := s.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("access: load user: %w", err)
	}
	if !row.IsActive {
		return User{}, ErrUserNotFound
	}
	return User{
		ID:                row.ID,
		Username:          row.Username,
		FullName:          row.FullName,
		Role:              Role(row.Role),
		IsActive:          row.IsActive,
		PasswordChangedAt: timePtr(row.PasswordChangedAt),
	}, nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
