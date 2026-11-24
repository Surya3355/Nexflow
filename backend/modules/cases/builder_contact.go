package cases

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Builder contact errors (spec 2026-11-24 §4.3).
var (
	ErrBuilderContactRequired = errors.New("cases: choose a builder contact")
	ErrBuilderContactInvalid  = errors.New("cases: pick a current contact of this project")
)

// BuilderContact is the project contact a case points to. Removed means
// admin took it off the project's list after it was chosen.
type BuilderContact struct {
	ID                        int64
	Name, Phone, Email, Label string
	Removed                   bool
}

// checkContact applies the Add case rule: a project with active contacts
// needs one of them; a project with none (NA or an old project) takes none.
func (s *Service) checkContact(ctx context.Context, q *casesdb.Queries, projectID, contactID int64) (pgtype.Int8, error) {
	has, err := q.ProjectHasActiveContact(ctx, projectID)
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("cases: project contacts: %w", err)
	}
	if contactID == 0 {
		if has {
			return pgtype.Int8{}, ErrBuilderContactRequired
		}
		return pgtype.Int8{}, nil
	}
	ok, err := q.ActiveContactInProject(ctx, casesdb.ActiveContactInProjectParams{ID: contactID, ProjectID: projectID})
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("cases: check contact: %w", err)
	}
	if !ok {
		return pgtype.Int8{}, ErrBuilderContactInvalid
	}
	return pgtype.Int8{Int64: contactID, Valid: true}, nil
}

// SetBuilderContact points a case at another active contact of its project.
// All staff may do this; a dropped case cannot change.
func (s *Service) SetBuilderContact(ctx context.Context, actor access.User, caseID, contactID int64) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
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
	if contactID == 0 {
		return Case{}, ErrBuilderContactInvalid
	}
	to, err := s.checkContact(txCtx, q, locked.ProjectID, contactID)
	if err != nil {
		return Case{}, err
	}
	if !locked.BuilderContactID.Valid || locked.BuilderContactID.Int64 != contactID {
		if err := q.SetCaseBuilderContact(txCtx, casesdb.SetCaseBuilderContactParams{
			ID: caseID, BuilderContactID: to, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: set contact: %w", err)
		}
		actorID, id := actor.ID, caseID
		if err := s.rec.Record(txCtx, history.Entry{
			ActorUserID: &actorID, Action: "case.builder_contact_changed", EntityType: "case", EntityID: &id,
			Details: map[string]any{"from": int8OrNil(locked.BuilderContactID), "to": contactID},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: audit: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

// int8OrNil is the id for audit details, or nil when unset.
func int8OrNil(v pgtype.Int8) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
