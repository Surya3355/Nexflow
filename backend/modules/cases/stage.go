package cases

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Errors for stage moves and case lookups.
var (
	ErrTrackingRequired  = errors.New("cases: tracking number is required when the case moves to Logged in")
	ErrCaseDropped       = errors.New("cases: a dropped case cannot change stage; restore it first")
	ErrCaseNotFound      = errors.New("cases: case not found")
	ErrStageNotForBank   = errors.New("cases: stage does not belong to this bank")
	ErrDuplicateTracking = errors.New("cases: tracking number is already used by a live case")
)

// Stage codes that carry rules (spec §5).
const (
	stageLoggedIn  = "logged_in"
	stageDisbursed = "disbursed"
)

// istZone is India Standard Time. "Today" for dates in this app is IST (spec §9).
var istZone = time.FixedZone("IST", 5*60*60+30*60)

// SanctionDetails are captured when a case is sanctioned (spec §4.2, round 8).
type SanctionDetails struct {
	Amount       string
	RatePercent  string
	InterestType string // "fixed" or "floating"
	TenureMonths int
	Date         time.Time
}

// DisbursementDetails are captured when a case is disbursed (spec §4.2).
type DisbursementDetails struct {
	Amount string
	Date   time.Time
}

// ChangeStage moves a case to another stage of its bank. Any stage can be
// reached, forward or backward (spec §5, round 8). Every move adds a dated
// history row and an audit entry, in one transaction.
//
// Rules: a dropped case cannot move. Moving to Logged in needs a tracking
// number (unless the case already has one). Moving to Disbursed completes the case.
func (s *Service) ChangeStage(ctx context.Context, actor access.User, caseID, toStageID int64,
	trackingNumber *string, sanction *SanctionDetails, disbursement *DisbursementDetails) (Case, error) {
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

	stages, err := s.workflows.StagesFor(txCtx, locked.BankID)
	if err != nil {
		return Case{}, err
	}
	var to *struct {
		id   int64
		code string
	}
	for _, st := range stages {
		if st.ID == toStageID {
			to = &struct {
				id   int64
				code string
			}{st.ID, st.Code}
			break
		}
	}
	if to == nil {
		return Case{}, ErrStageNotForBank
	}

	var newTracking pgtype.Text
	if trackingNumber != nil {
		v, err := NormalizeTracking(*trackingNumber)
		if err != nil {
			return Case{}, err
		}
		if v != "" {
			newTracking = pgtype.Text{String: v, Valid: true}
		}
	}
	hasTracking := newTracking.Valid || (locked.TrackingNumber.Valid && locked.TrackingNumber.String != "")
	if to.code == stageLoggedIn && !hasTracking {
		return Case{}, ErrTrackingRequired
	}

	status := "active"
	if to.code == stageDisbursed {
		status = "completed"
	}
	if err := q.SetCaseStage(txCtx, casesdb.SetCaseStageParams{
		ID: caseID, CurrentStageID: to.id, Status: status,
		UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true}, TrackingNumber: newTracking,
	}); err != nil {
		if isUniqueViolation(err) {
			return Case{}, ErrDuplicateTracking
		}
		return Case{}, fmt.Errorf("cases: set stage: %w", err)
	}
	if sanction != nil {
		if err := setSanction(txCtx, q, caseID, actor.ID, *sanction); err != nil {
			return Case{}, err
		}
	}
	if disbursement != nil {
		if err := setDisbursement(txCtx, q, caseID, actor.ID, *disbursement); err != nil {
			return Case{}, err
		}
	}

	now := time.Now()
	if err := q.InsertCaseStageEventAt(txCtx, casesdb.InsertCaseStageEventAtParams{
		CaseID: caseID, StageID: to.id, ReachedAt: pgtype.Timestamptz{Time: now, Valid: true}, ChangedBy: actor.ID,
	}); err != nil {
		return Case{}, fmt.Errorf("cases: stage event: %w", err)
	}

	// Document purge: one year after the case is disbursed (spec §4.4).
	if status == "completed" {
		purge := istDate(now).AddDate(1, 0, 0)
		if err := q.SetCaseDocumentsPurgeAfter(txCtx, casesdb.SetCaseDocumentsPurgeAfterParams{
			CaseID: caseID, PurgeAfter: pgtype.Date{Time: purge, Valid: true},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: purge date: %w", err)
		}
	} else if locked.Status == "completed" {
		if err := q.ClearCaseDocumentsPurgeAfter(txCtx, caseID); err != nil {
			return Case{}, fmt.Errorf("cases: clear purge date: %w", err)
		}
	}

	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.stage_changed", EntityType: "case", EntityID: &id,
		Details: map[string]any{"to_stage_id": to.id, "to_stage_code": to.code},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}

	row, err := q.GetCase(txCtx, caseID)
	if err != nil {
		return Case{}, fmt.Errorf("cases: reload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return toCase(row), nil
}

func setSanction(ctx context.Context, q *casesdb.Queries, caseID, actorID int64, d SanctionDetails) error {
	amount, err := parseAmount(d.Amount)
	if err != nil {
		return err
	}
	rate := pgtype.Numeric{}
	if err := rate.Scan(d.RatePercent); err != nil {
		return fmt.Errorf("cases: interest rate: %w", err)
	}
	var itype pgtype.Text
	if d.InterestType != "" {
		itype = pgtype.Text{String: d.InterestType, Valid: true}
	}
	var tenure pgtype.Int4
	if d.TenureMonths > 0 {
		tenure = pgtype.Int4{Int32: int32(d.TenureMonths), Valid: true}
	}
	return q.SetCaseSanction(ctx, casesdb.SetCaseSanctionParams{
		ID: caseID, SanctionedAmount: amount, InterestRate: rate, InterestType: itype,
		TenureMonths: tenure, SanctionDate: pgtype.Date{Time: istDate(d.Date), Valid: !d.Date.IsZero()},
		UpdatedBy: pgtype.Int8{Int64: actorID, Valid: true},
	})
}

func setDisbursement(ctx context.Context, q *casesdb.Queries, caseID, actorID int64, d DisbursementDetails) error {
	amount, err := parseAmount(d.Amount)
	if err != nil {
		return err
	}
	return q.SetCaseDisbursement(ctx, casesdb.SetCaseDisbursementParams{
		ID: caseID, DisbursedAmount: amount,
		DisbursementDate: pgtype.Date{Time: istDate(d.Date), Valid: !d.Date.IsZero()},
		UpdatedBy:        pgtype.Int8{Int64: actorID, Valid: true},
	})
}

// istDate returns the IST calendar date of t, as a date at midnight UTC.
func istDate(t time.Time) time.Time {
	local := t.In(istZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func numericString(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(driver.Value(v))
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}
