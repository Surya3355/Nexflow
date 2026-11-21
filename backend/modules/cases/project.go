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

// ChangeProject moves a case to another project. All staff may edit. A
// dropped case cannot change. The change is audited with the old and new
// project; choosing the current project changes nothing.
func (s *Service) ChangeProject(ctx context.Context, actor access.User, caseID, projectID int64) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	if err := s.checkProject(ctx, projectID); err != nil {
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
	if locked.ProjectID != projectID {
		if err := q.UpdateCaseProject(txCtx, casesdb.UpdateCaseProjectParams{
			ID: caseID, ProjectID: projectID, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: update project: %w", err)
		}
		actorID, id := actor.ID, caseID
		if err := s.rec.Record(txCtx, history.Entry{
			ActorUserID: &actorID, Action: "case.project_changed", EntityType: "case", EntityID: &id,
			Details: map[string]int64{"from": locked.ProjectID, "to": projectID},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: audit: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}
