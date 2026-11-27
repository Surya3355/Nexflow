package cases

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// noOpenQueries refuses a change while the case has bank queries not yet
// resolved: moving forward or completing the case (spec 2026-11-26 bank queries §5.3).
func noOpenQueries(ctx context.Context, q *casesdb.Queries, caseID int64) error {
	n, err := q.CountOpenCaseQueries(ctx, caseID)
	if err != nil {
		return fmt.Errorf("cases: open queries: %w", err)
	}
	if n > 0 {
		return ErrOpenQueries
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Errors for stage moves and case lookups.
var (
	ErrTrackingRequired       = errors.New("cases: tracking number is required at this stage")
	ErrCaseDropped            = errors.New("cases: a dropped case cannot change stage; restore it first")
	ErrCaseNotFound           = errors.New("cases: case not found")
	ErrStageNotForBank        = errors.New("cases: stage does not belong to this bank")
	ErrDuplicateTracking      = errors.New("cases: tracking number is already used by a live case")
	ErrSanctionAmountRequired = errors.New("cases: enter the sanctioned amount")
	ErrRemarkTooLong          = errors.New("cases: remark must be 500 characters or fewer")
	ErrOpenQueries            = errors.New("cases: resolve the open bank queries first")
)

// maxRemark is the longest remark a stage move can carry.
const maxRemark = 500

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

// StageMove is one stage change as staff ask for it.
type StageMove struct {
	ToStageID      int64
	TrackingNumber *string          // needed to reach the tracking-role stage
	Sanction       *SanctionDetails // needed to reach the sanction-role stage
	Payment        *PaymentInput    // first payment, when reaching the disbursement-role stage
	Remark         string           // optional
	OfficerID      int64            // officer for the target stage's team; 0 keeps the current one
}

// ChangeStage moves a case to another stage of its bank. Any stage can be
// reached, forward or backward (spec §5, round 8). Every move adds a dated
// history row and an audit entry, in one transaction.
//
// Rules by stage role (spec 2026-11-25 §5.3): the tracking stage needs a
// tracking number; the sanction stage needs the sanctioned amount, which
// replaces the loan amount; the disbursement stage needs a first payment
// unless the case already has one. A case is completed only while it is at
// the disbursement stage and has its Full payment. A dropped case cannot move.
func (s *Service) ChangeStage(ctx context.Context, actor access.User, caseID int64, m StageMove) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	remark := strings.TrimSpace(m.Remark)
	if utf8.RuneCountInString(remark) > maxRemark {
		return Case{}, ErrRemarkTooLong
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
	var to *workflows.Stage
	for i := range stages {
		if stages[i].ID == m.ToStageID {
			to = &stages[i]
			break
		}
	}
	if to == nil {
		return Case{}, ErrStageNotForBank
	}
	if m.OfficerID != 0 && to.OfficerTeam == "" {
		return Case{}, ErrOfficerInvalid
	}
	// Open bank queries stop the case moving forward; back is always allowed
	// (spec 2026-11-26 bank queries §5.3).
	for _, cur := range stages {
		if cur.ID == locked.CurrentStageID && to.SortOrder > cur.SortOrder {
			if err := noOpenQueries(txCtx, q, caseID); err != nil {
				return Case{}, err
			}
		}
	}

	var newTracking pgtype.Text
	if m.TrackingNumber != nil {
		v, err := NormalizeTracking(*m.TrackingNumber)
		if err != nil {
			return Case{}, err
		}
		if v != "" {
			newTracking = pgtype.Text{String: v, Valid: true}
		}
	}
	hasTracking := newTracking.Valid || (locked.TrackingNumber.Valid && locked.TrackingNumber.String != "")
	if to.Role == workflows.RoleTracking && !hasTracking {
		return Case{}, ErrTrackingRequired
	}
	if to.Role == workflows.RoleSanction && (m.Sanction == nil || strings.TrimSpace(m.Sanction.Amount) == "") {
		return Case{}, ErrSanctionAmountRequired
	}

	fullyPaid := false
	if to.Role == workflows.RoleDisbursement {
		state, err := q.CasePaymentState(txCtx, caseID)
		if err != nil {
			return Case{}, fmt.Errorf("cases: payment state: %w", err)
		}
		fullyPaid = state.FullyPaid
		if m.Payment == nil && state.Payments == 0 {
			return Case{}, ErrPaymentRequired
		}
		if m.Payment != nil {
			full, err := s.insertPaymentTx(txCtx, q, actor, caseID, locked.BankID, *m.Payment)
			if err != nil {
				return Case{}, err
			}
			fullyPaid = full
			if err := s.audit(txCtx, actor, "case.payment_added", caseID, map[string]any{
				"amount": m.Payment.Amount, "kind": m.Payment.Kind,
				"paid_on": s.auditDay(m.Payment.Date)}); err != nil {
				return Case{}, err
			}
		}
	}

	status := "active"
	if fullyPaid {
		status = "completed"
		if err := noOpenQueries(txCtx, q, caseID); err != nil {
			return Case{}, err
		}
	}
	if err := q.SetCaseStage(txCtx, casesdb.SetCaseStageParams{
		ID: caseID, CurrentStageID: to.ID, Status: status,
		UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true}, TrackingNumber: newTracking,
	}); err != nil {
		if isUniqueViolation(err) {
			return Case{}, ErrDuplicateTracking
		}
		return Case{}, fmt.Errorf("cases: set stage: %w", err)
	}
	details := map[string]any{"to_stage_id": to.ID, "to_stage_code": to.Code}
	if to.Role == workflows.RoleSanction {
		details["old_loan_amount"] = numericString(locked.LoanAmount)
		if err := setSanction(txCtx, q, caseID, actor.ID, *m.Sanction); err != nil {
			return Case{}, err
		}
	}
	if remark != "" {
		details["remark"] = remark
	}

	now := time.Now()
	if err := q.InsertCaseStageEventAt(txCtx, casesdb.InsertCaseStageEventAtParams{
		CaseID: caseID, StageID: to.ID, ReachedAt: pgtype.Timestamptz{Time: now, Valid: true}, ChangedBy: actor.ID,
		Remark: pgtype.Text{String: remark, Valid: remark != ""},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: stage event: %w", err)
	}
	if m.OfficerID != 0 {
		if err := s.pickOfficerTx(txCtx, q, actor, caseID, locked.BankID, to.OfficerTeam, m.OfficerID, now); err != nil {
			return Case{}, err
		}
	}
	if err := s.syncPurgeDate(txCtx, q, caseID, locked.Status, status, now); err != nil {
		return Case{}, err
	}

	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.stage_changed", EntityType: "case", EntityID: &id, Details: details,
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

// syncPurgeDate sets the document purge date when a case becomes completed
// (one year after, spec §4.4) and clears it when a completed case reopens.
func (s *Service) syncPurgeDate(ctx context.Context, q *casesdb.Queries, caseID int64, was, now string, at time.Time) error {
	switch {
	case now == "completed" && was != "completed":
		purge := istDate(at).AddDate(1, 0, 0)
		if err := q.SetCaseDocumentsPurgeAfter(ctx, casesdb.SetCaseDocumentsPurgeAfterParams{
			CaseID: caseID, PurgeAfter: pgtype.Date{Time: purge, Valid: true},
		}); err != nil {
			return fmt.Errorf("cases: purge date: %w", err)
		}
	case now != "completed" && was == "completed":
		if err := q.ClearCaseDocumentsPurgeAfter(ctx, caseID); err != nil {
			return fmt.Errorf("cases: clear purge date: %w", err)
		}
	}
	return nil
}

func setSanction(ctx context.Context, q *casesdb.Queries, caseID, actorID int64, d SanctionDetails) error {
	amount, err := parseAmount(d.Amount)
	if err != nil {
		return err
	}
	rate, err := parseRate(d.RatePercent)
	if err != nil {
		return err
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

// ErrInvalidRate refuses an interest rate that isn't a percentage.
var ErrInvalidRate = errors.New("cases: enter the interest rate as a percentage, like 8.5")

var ratePattern = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,2})?$`)

// parseRate reads an interest rate in percent: empty means not given;
// otherwise more than 0 and at most 100, with at most 2 decimals.
func parseRate(raw string) (pgtype.Numeric, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Numeric{}, nil
	}
	if !ratePattern.MatchString(raw) {
		return pgtype.Numeric{}, ErrInvalidRate
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 || f > 100 {
		return pgtype.Numeric{}, ErrInvalidRate
	}
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		return pgtype.Numeric{}, ErrInvalidRate
	}
	return n, nil
}
