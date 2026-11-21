package cases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors for drops.
var (
	ErrInvalidReason  = errors.New("cases: choose a valid drop reason")
	ErrRemarkRequired = errors.New("cases: a remark is required when the reason is Other")
	ErrAlreadyDropped = errors.New("cases: case is already dropped")
)

// otherReason is the reason that needs a remark (spec §4.2).
const otherReason = "Other"

// DropInput is the reason for dropping a case, from the admin list (spec §4.2).
type DropInput struct {
	ReasonID int64
	Remark   string
}

// Drop closes a case with a reason. The case stays visible with status
// Dropped and keeps its stage history. Its documents are purged one year later
// (spec §4.4). Any staff member may drop (spec §6).
func (s *Service) Drop(ctx context.Context, actor access.User, caseID int64, in DropInput) (Case, error) {
	if !access.Can(actor.Role, access.ActionDropRecord) {
		return Case{}, access.ErrForbidden
	}
	reason, err := s.dropReason(ctx, in)
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
		return Case{}, ErrAlreadyDropped
	}

	if err := q.DropCase(txCtx, casesdb.DropCaseParams{
		ID: caseID, DroppedReasonID: pgtype.Int8{Int64: reason.ID, Valid: true},
		DroppedRemark: textOrNull(in.Remark), DroppedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: drop: %w", err)
	}
	if err := q.SetCaseDocumentsPurgeAfter(txCtx, casesdb.SetCaseDocumentsPurgeAfterParams{
		CaseID: caseID, PurgeAfter: pgtype.Date{Time: istDate(time.Now()).AddDate(1, 0, 0), Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: purge date: %w", err)
	}

	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.dropped", EntityType: "case", EntityID: &id,
		Details: map[string]any{"reason_id": reason.ID, "reason": reason.Value},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

// dropReason checks that the reason exists and that a remark is given for "Other".
func (s *Service) dropReason(ctx context.Context, in DropInput) (casesdb.GetDropReasonRow, error) {
	if in.ReasonID == 0 {
		return casesdb.GetDropReasonRow{}, ErrInvalidReason
	}
	reason, err := s.q.GetDropReason(ctx, in.ReasonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return casesdb.GetDropReasonRow{}, ErrInvalidReason
	}
	if err != nil {
		return casesdb.GetDropReasonRow{}, fmt.Errorf("cases: reason: %w", err)
	}
	if reason.Value == otherReason && strings.TrimSpace(in.Remark) == "" {
		return casesdb.GetDropReasonRow{}, ErrRemarkRequired
	}
	return reason, nil
}

func textOrNull(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}
