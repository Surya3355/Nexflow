package cases

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgtype"
)

// CaseFields are the editable case fields. Stage, assignee and status have
// their own actions and cannot be edited here (spec §4.2, plan Task 15).
// FollowUpDate nil clears the follow-up date (the optional date, spec §4.2).
type CaseFields struct {
	LoanAmount   string
	FollowUpDate *time.Time
}

// Update edits the loan amount and follow-up date of a case. All staff may edit.
func (s *Service) Update(ctx context.Context, actor access.User, caseID int64, in CaseFields) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	amount, err := parseAmount(in.LoanAmount)
	if err != nil {
		return Case{}, err
	}
	if _, err := s.load(ctx, caseID); err != nil {
		return Case{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("cases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txCtx := history.WithTx(ctx, tx)
	q := s.q.WithTx(tx)

	var follow pgtype.Date
	if in.FollowUpDate != nil {
		follow = pgtype.Date{Time: istDate(*in.FollowUpDate), Valid: true}
	}
	if err := q.UpdateCaseFields(txCtx, casesdb.UpdateCaseFieldsParams{
		ID: caseID, LoanAmount: amount, FollowUpDate: follow,
		UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: update: %w", err)
	}
	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.updated", EntityType: "case", EntityID: &id,
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}
