package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access/accessdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the access service. Callers compare with errors.Is.
var (
	ErrForbidden          = errors.New("access: not allowed for this role")
	ErrUsernameTaken      = errors.New("access: username is already in use")
	ErrInvalidUsername    = errors.New("access: username is required")
	ErrInvalidRole        = errors.New("access: unknown role")
	ErrWeakPassword       = errors.New("access: password must be at least 8 characters")
	ErrInvalidCredentials = errors.New("access: invalid username or password")
	ErrUserNotFound       = errors.New("access: user not found")
)

// minPasswordLen is a security floor. The spec does not state a rule; this is
// recorded as a ruling in the plan ledger.
const minPasswordLen = 8

// User is a staff account as the application sees it. Never holds the hash.
type User struct {
	ID                int64
	Username          string
	FullName          string
	Role              Role
	IsActive          bool
	PasswordChangedAt *time.Time
}

// NewUser is the input for admin-created accounts. Email is optional and is
// used for alert emails (spec §10).
type NewUser struct {
	Username string
	FullName string
	Role     Role
	Password string
	Email    string
}

// Service holds the database handle and the audit recorder for access actions.
type Service struct {
	pool    *pgxpool.Pool
	q       *accessdb.Queries
	rec     history.Recorder
	clock   func() time.Time
	unknown *unknownLocks
	// reassigner moves open work when a user is switched off (deactivate.go).
	reassigner Reassigner
}

// NewService returns an access service on pool that writes audit entries to rec.
func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service {
	return &Service{pool: pool, q: accessdb.New(pool), rec: rec, clock: time.Now, unknown: newUnknownLocks()}
}

// SetClock replaces the clock used by the login lock. Tests use it.
func (s *Service) SetClock(clock func() time.Time) { s.clock = clock }

// CreateUser adds a staff account. Only an actor who may manage users can do it.
// The username is stored lowercase and must be unique.
func (s *Service) CreateUser(ctx context.Context, actor User, in NewUser) (User, error) {
	if !Can(actor.Role, ActionManageUsers) {
		return User{}, ErrForbidden
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if username == "" {
		return User{}, ErrInvalidUsername
	}
	if !in.Role.Valid() {
		return User{}, ErrInvalidRole
	}
	if len(in.Password) < minPasswordLen {
		return User{}, ErrWeakPassword
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, err
	}

	row, err := s.q.InsertUser(ctx, accessdb.InsertUserParams{
		Username:     username,
		FullName:     in.FullName,
		PasswordHash: hash,
		Role:         string(in.Role),
		Email:        textOrNull(in.Email),
		CreatedBy:    actorRef(actor.ID),
		UpdatedBy:    actorRef(actor.ID),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("access: insert user: %w", err)
	}

	u := User{
		ID:       row.ID,
		Username: row.Username,
		FullName: row.FullName,
		Role:     Role(row.Role),
		IsActive: row.IsActive,
	}
	if row.PasswordChangedAt.Valid {
		t := row.PasswordChangedAt.Time
		u.PasswordChangedAt = &t
	}
	var actorID *int64
	if actor.ID > 0 {
		a := actor.ID
		actorID = &a
	}
	id := u.ID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: actorID,
		Action:      "user.created",
		EntityType:  "user",
		EntityID:    &id,
		Details:     map[string]string{"username": u.Username, "role": string(u.Role)},
	}); err != nil {
		return User{}, fmt.Errorf("access: audit: %w", err)
	}
	return u, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ChangePassword lets a signed-in user change their own password. The current
// password must match. Sets password_changed_at and writes an audit entry.
func (s *Service) ChangePassword(ctx context.Context, actor User, current, next string) error {
	row, err := s.q.GetUserByID(ctx, actor.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("access: load user: %w", err)
	}
	ok, err := VerifyPassword(row.PasswordHash, current)
	if err != nil {
		return fmt.Errorf("access: verify: %w", err)
	}
	if !ok {
		return ErrInvalidCredentials
	}
	if len(next) < minPasswordLen {
		return ErrWeakPassword
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.q.UpdateUserPassword(ctx, accessdb.UpdateUserPasswordParams{
		ID:           actor.ID,
		PasswordHash: hash,
		UpdatedBy:    pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return fmt.Errorf("access: update password: %w", err)
	}
	id := actor.ID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &id,
		Action:      "user.password_changed",
		EntityType:  "user",
		EntityID:    &id,
	}); err != nil {
		return fmt.Errorf("access: audit: %w", err)
	}
	return nil
}

// ResetPassword lets an admin set a new password for any user, without the
// current one. Sessions the user already has stop working (see RequireUser).
func (s *Service) ResetPassword(ctx context.Context, actor User, userID int64, next string) error {
	if !Can(actor.Role, ActionManageUsers) {
		return ErrForbidden
	}
	if len(next) < minPasswordLen {
		return ErrWeakPassword
	}
	if _, err := s.q.GetUserByID(ctx, userID); errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	} else if err != nil {
		return fmt.Errorf("access: load user: %w", err)
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.q.UpdateUserPassword(ctx, accessdb.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: hash,
		UpdatedBy:    pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return fmt.Errorf("access: reset password: %w", err)
	}
	actorID, id := actor.ID, userID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID,
		Action:      "user.password_reset",
		EntityType:  "user",
		EntityID:    &id,
	}); err != nil {
		return fmt.Errorf("access: audit: %w", err)
	}
	return nil
}

// ListActiveUsers returns active staff for pickers (assignee filter, reassignment).
func (s *Service) ListActiveUsers(ctx context.Context) ([]User, error) {
	rows, err := s.q.ListActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("access: list users: %w", err)
	}
	out := make([]User, 0, len(rows))
	for _, r := range rows {
		out = append(out, User{ID: r.ID, Username: r.Username, FullName: r.FullName, Role: Role(r.Role), IsActive: true})
	}
	return out, nil
}

func textOrNull(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

// ErrUsersExist is returned by CreateFirstAdmin when the database already has users.
var ErrUsersExist = errors.New("access: users already exist; use an admin account to add more")

// CreateFirstAdmin creates the first admin account. It works only on an empty
// users table, so it cannot be used to add accounts later.
func CreateFirstAdmin(ctx context.Context, pool *pgxpool.Pool, username, fullName, password string) (User, error) {
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return User{}, fmt.Errorf("access: count users: %w", err)
	}
	if n > 0 {
		return User{}, ErrUsersExist
	}
	svc := NewService(pool, history.Nop{})
	bootstrap := User{Role: RoleAdmin, IsActive: true}
	created, err := svc.CreateUser(ctx, bootstrap, NewUser{Username: username, FullName: fullName, Role: RoleAdmin, Password: password})
	if err != nil {
		return User{}, err
	}
	return created, nil
}

// actorRef is the user ID to store on a row, or NULL for the bootstrap admin, who has no creator.
func actorRef(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
