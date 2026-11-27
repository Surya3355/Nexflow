package cases

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrUnitRequired refuses an empty unit number on a builder project (one with
// a type). NA and old untyped projects stay optional (spec 2026-11-24 unit label).
var ErrUnitRequired = errors.New("cases: enter the unit number for this project")

// ErrInvalidUnit refuses a unit number outside the allowed characters or length.
var ErrInvalidUnit = errors.New("cases: unit no. can have letters, numbers, spaces, - and /, up to 20 characters")

var unitPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9 /-]{0,19}$`)

// NormalizeUnit trims and upper-cases a flat, villa or plot number. Empty means none.
func NormalizeUnit(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	if v == "" {
		return "", nil
	}
	if !unitPattern.MatchString(v) {
		return "", ErrInvalidUnit
	}
	return v, nil
}

// SetUnitNumber changes or clears a case's unit number. A dropped case is refused.
func (s *Service) SetUnitNumber(ctx context.Context, actor access.User, caseID int64, unit string) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	v, err := NormalizeUnit(unit)
	if err != nil {
		return Case{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("cases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)
	locked, err := q.LockCase(txCtx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: lock: %w", err)
	}
	if locked.Status == "dropped" {
		return Case{}, ErrCaseDropped
	}
	if v == "" {
		if err := s.checkUnitRequired(txCtx, q, locked.ProjectID, v); err != nil {
			return Case{}, err
		}
	}
	if err := q.SetCaseUnit(txCtx, casesdb.SetCaseUnitParams{
		ID: caseID, UnitNumber: pgtype.Text{String: v, Valid: v != ""}, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: set unit: %w", err)
	}
	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.unit_changed", EntityType: "case", EntityID: &id,
		Details: map[string]any{"unit_number": v},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

// checkUnitRequired refuses an empty unit on a project that has a type.
func (s *Service) checkUnitRequired(ctx context.Context, q *casesdb.Queries, projectID int64, unit string) error {
	if unit != "" {
		return nil
	}
	typ, err := q.ProjectType(ctx, projectID)
	if err != nil {
		return fmt.Errorf("cases: project type: %w", err)
	}
	if typ.Valid {
		return ErrUnitRequired
	}
	return nil
}
