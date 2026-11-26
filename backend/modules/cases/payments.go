package cases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Payment kinds (spec 2026-11-25 §5.2).
const (
	PaymentPart = "part"
	PaymentFull = "full"
)

// Errors for disbursement payments.
var (
	ErrPartNotAllowed     = errors.New("cases: this bank does not pay in parts; record a Full payment")
	ErrAlreadyFullyPaid   = errors.New("cases: this case already has its Full payment")
	ErrInvalidPaymentKind = errors.New("cases: payment must be Part or Full")
	ErrPaymentRequired    = errors.New("cases: enter the first payment to move to disbursement")
)

// PaymentInput is one disbursement payment as staff enter it.
type PaymentInput struct {
	Amount string
	Date   time.Time // zero means today (IST)
	Kind   string    // PaymentPart or PaymentFull
}

// insertPaymentTx records one payment inside the caller's transaction, with
// the case row already locked. It reports whether the case is now fully paid.
func (s *Service) insertPaymentTx(ctx context.Context, q *casesdb.Queries, actor access.User, caseID, bankID int64, p PaymentInput) (bool, error) {
	if p.Kind != PaymentPart && p.Kind != PaymentFull {
		return false, ErrInvalidPaymentKind
	}
	amount, err := parseAmount(p.Amount)
	if err != nil {
		return false, err
	}
	if p.Kind == PaymentPart {
		on, err := s.workflows.BankPartPayments(ctx, bankID)
		if err != nil {
			return false, err
		}
		if !on {
			return false, ErrPartNotAllowed
		}
	}
	state, err := q.CasePaymentState(ctx, caseID)
	if err != nil {
		return false, fmt.Errorf("cases: payment state: %w", err)
	}
	if state.FullyPaid {
		return false, ErrAlreadyFullyPaid
	}
	day := orToday(p.Date)
	if _, err := q.InsertCaseDisbursement(ctx, casesdb.InsertCaseDisbursementParams{
		CaseID: caseID, Amount: amount, PaidOn: pgtype.Date{Time: istDate(day), Valid: true},
		Kind: p.Kind, CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		if isUniqueViolation(err) {
			return false, ErrAlreadyFullyPaid
		}
		return false, fmt.Errorf("cases: insert payment: %w", err)
	}
	return p.Kind == PaymentFull, nil
}

// More errors for payments on an existing case.
var (
	ErrNotAtDisbursement = errors.New("cases: payments can be added only at the disbursement stage")
	ErrPaymentNotFound   = errors.New("cases: payment not found")
)

// Payment is one recorded disbursement payment.
type Payment struct {
	ID        int64
	Amount    string
	PaidOn    time.Time
	Kind      string
	CreatedBy int64
	CreatedAt time.Time
}

// PaymentSummary is what the case page shows (spec 2026-11-25 §5.4).
// Remaining = sanctioned amount (else loan amount) − total paid, never below 0.
type PaymentSummary struct {
	Payments  []Payment
	TotalPaid string
	Remaining string
	FullyPaid bool
	OverPaid  bool
}

// Payments returns a case's payments, oldest first, with the totals.
func (s *Service) Payments(ctx context.Context, caseID int64) (PaymentSummary, error) {
	return paymentsOf(ctx, s.q, caseID)
}

func paymentsOf(ctx context.Context, q *casesdb.Queries, caseID int64) (PaymentSummary, error) {
	tot, err := q.CasePaymentTotals(ctx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentSummary{}, ErrCaseNotFound
	}
	if err != nil {
		return PaymentSummary{}, fmt.Errorf("cases: payment totals: %w", err)
	}
	rows, err := q.ListCasePayments(ctx, caseID)
	if err != nil {
		return PaymentSummary{}, fmt.Errorf("cases: payments: %w", err)
	}
	out := PaymentSummary{Payments: make([]Payment, 0, len(rows)), TotalPaid: tot.TotalPaid,
		Remaining: tot.Remaining, FullyPaid: tot.FullyPaid, OverPaid: tot.OverPaid}
	for _, r := range rows {
		out.Payments = append(out.Payments, Payment{ID: r.ID, Amount: r.Amount, PaidOn: r.PaidOn.Time,
			Kind: r.Kind, CreatedBy: r.CreatedBy.Int64, CreatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

// AddPayment records a payment on a case at its disbursement stage. A Full
// payment completes the case.
func (s *Service) AddPayment(ctx context.Context, actor access.User, caseID int64, p PaymentInput) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	return s.paymentTx(ctx, actor, caseID, func(txCtx context.Context, q *casesdb.Queries, locked casesdb.LockCaseRow) error {
		role, err := q.CaseStageRole(txCtx, caseID)
		if err != nil {
			return fmt.Errorf("cases: stage role: %w", err)
		}
		if role != workflows.RoleDisbursement {
			return ErrNotAtDisbursement
		}
		if _, err := s.insertPaymentTx(txCtx, q, actor, caseID, locked.BankID, p); err != nil {
			return err
		}
		return s.audit(txCtx, actor, "case.payment_added", caseID, map[string]any{
			"amount": p.Amount, "kind": p.Kind, "paid_on": istDate(orToday(p.Date)).Format(time.DateOnly)})
	})
}

// UpdatePayment corrects a recorded payment. Admin and manager only.
func (s *Service) UpdatePayment(ctx context.Context, actor access.User, caseID, paymentID int64, p PaymentInput) (Case, error) {
	if !access.Can(actor.Role, access.ActionCorrectPayment) {
		return Case{}, access.ErrForbidden
	}
	if p.Kind != PaymentPart && p.Kind != PaymentFull {
		return Case{}, ErrInvalidPaymentKind
	}
	amount, err := parseAmount(p.Amount)
	if err != nil {
		return Case{}, err
	}
	return s.paymentTx(ctx, actor, caseID, func(txCtx context.Context, q *casesdb.Queries, locked casesdb.LockCaseRow) error {
		before, err := q.GetCasePayment(txCtx, casesdb.GetCasePaymentParams{ID: paymentID, CaseID: caseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("cases: payment: %w", err)
		}
		if p.Kind == PaymentPart && before.Kind != PaymentPart {
			on, err := s.workflows.BankPartPayments(txCtx, locked.BankID)
			if err != nil {
				return err
			}
			if !on {
				return ErrPartNotAllowed
			}
		}
		day := istDate(orToday(p.Date))
		if err := q.UpdateCasePayment(txCtx, casesdb.UpdateCasePaymentParams{
			ID: paymentID, CaseID: caseID, Amount: amount, PaidOn: pgtype.Date{Time: day, Valid: true},
			Kind: p.Kind, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		}); err != nil {
			if isUniqueViolation(err) {
				return ErrAlreadyFullyPaid
			}
			return fmt.Errorf("cases: update payment: %w", err)
		}
		return s.audit(txCtx, actor, "case.payment_updated", caseID, map[string]any{
			"payment_id": paymentID, "before": paymentDetails(before),
			"after": map[string]any{"amount": numericString(amount), "kind": p.Kind, "paid_on": day.Format(time.DateOnly)}})
	})
}

// DeletePayment removes a wrong payment. Admin and manager only. Removing the
// Full payment reopens a completed case.
func (s *Service) DeletePayment(ctx context.Context, actor access.User, caseID, paymentID int64) (Case, error) {
	if !access.Can(actor.Role, access.ActionCorrectPayment) {
		return Case{}, access.ErrForbidden
	}
	return s.paymentTx(ctx, actor, caseID, func(txCtx context.Context, q *casesdb.Queries, _ casesdb.LockCaseRow) error {
		before, err := q.GetCasePayment(txCtx, casesdb.GetCasePaymentParams{ID: paymentID, CaseID: caseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("cases: payment: %w", err)
		}
		if err := q.DeleteCasePayment(txCtx, casesdb.DeleteCasePaymentParams{ID: paymentID, CaseID: caseID}); err != nil {
			return fmt.Errorf("cases: delete payment: %w", err)
		}
		return s.audit(txCtx, actor, "case.payment_deleted", caseID, map[string]any{
			"payment_id": paymentID, "before": paymentDetails(before)})
	})
}

// paymentTx locks the case, runs change, then sets the case completed when it
// is at its disbursement stage with a Full payment, or active otherwise.
func (s *Service) paymentTx(ctx context.Context, actor access.User, caseID int64,
	change func(context.Context, *casesdb.Queries, casesdb.LockCaseRow) error) (Case, error) {
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
	if err := change(txCtx, q, locked); err != nil {
		return Case{}, err
	}
	state, err := q.CasePaymentState(txCtx, caseID)
	if err != nil {
		return Case{}, fmt.Errorf("cases: payment state: %w", err)
	}
	role, err := q.CaseStageRole(txCtx, caseID)
	if err != nil {
		return Case{}, fmt.Errorf("cases: stage role: %w", err)
	}
	status := "active"
	if state.FullyPaid && role == workflows.RoleDisbursement {
		status = "completed"
	}
	if status == "completed" && locked.Status != "completed" {
		if err := noOpenQueries(txCtx, q, caseID); err != nil {
			return Case{}, err
		}
	}
	if status != locked.Status {
		if err := q.SetCaseStatus(txCtx, casesdb.SetCaseStatusParams{
			ID: caseID, Status: status, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		}); err != nil {
			return Case{}, fmt.Errorf("cases: status: %w", err)
		}
		if err := s.syncPurgeDate(txCtx, q, caseID, locked.Status, status, time.Now()); err != nil {
			return Case{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

func (s *Service) audit(ctx context.Context, actor access.User, action string, caseID int64, details map[string]any) error {
	actorID, id := actor.ID, caseID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID, Action: action, EntityType: "case", EntityID: &id, Details: details,
	}); err != nil {
		return fmt.Errorf("cases: audit: %w", err)
	}
	return nil
}

func paymentDetails(r casesdb.GetCasePaymentRow) map[string]any {
	return map[string]any{"amount": r.Amount, "kind": r.Kind, "paid_on": r.PaidOn.Time.Format(time.DateOnly)}
}

func orToday(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}
