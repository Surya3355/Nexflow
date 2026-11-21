package customers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers/customersdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors for customer drops.
var (
	ErrInvalidReason  = errors.New("customers: choose a valid drop reason")
	ErrRemarkRequired = errors.New("customers: a remark is required when the reason is Other")
	ErrAlreadyDropped = errors.New("customers: customer is already dropped")
)

// DropInput is the reason for dropping a customer (spec §4.1).
type DropInput struct {
	ReasonID int64
	Remark   string
}

var istZone = time.FixedZone("IST", 5*60*60+30*60)

// Drop closes a customer and every live case it has, with the same reason
// (spec §4.1, round 9). Cases already dropped keep their own reason (owner
// answer A). Documents of the affected cases are purged one year later.
func (s *Service) Drop(ctx context.Context, actor access.User, customerID int64, in DropInput) (Customer, error) {
	if !access.Can(actor.Role, access.ActionDropRecord) {
		return Customer{}, access.ErrForbidden
	}
	if in.ReasonID == 0 {
		return Customer{}, ErrInvalidReason
	}
	reason, err := s.q.GetDropReason(ctx, in.ReasonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrInvalidReason
	}
	if err != nil {
		return Customer{}, fmt.Errorf("customers: reason: %w", err)
	}
	if reason.Value == "Other" && strings.TrimSpace(in.Remark) == "" {
		return Customer{}, ErrRemarkRequired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Customer{}, fmt.Errorf("customers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	locked, err := q.LockCustomer(txCtx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("customers: lock: %w", err)
	}
	if locked.Status == "dropped" {
		return Customer{}, ErrAlreadyDropped
	}

	remark := pgtype.Text{String: strings.TrimSpace(in.Remark), Valid: strings.TrimSpace(in.Remark) != ""}
	reasonParam := pgtype.Int8{Int64: reason.ID, Valid: true}
	by := pgtype.Int8{Int64: actor.ID, Valid: true}
	if err := q.DropCustomer(txCtx, customersdb.DropCustomerParams{
		ID: customerID, DroppedReasonID: reasonParam, DroppedRemark: remark, DroppedBy: by,
	}); err != nil {
		return Customer{}, fmt.Errorf("customers: drop: %w", err)
	}
	if err := q.DropCustomerCases(txCtx, customersdb.DropCustomerCasesParams{
		CustomerID: customerID, DroppedReasonID: reasonParam, DroppedRemark: remark, DroppedBy: by,
	}); err != nil {
		return Customer{}, fmt.Errorf("customers: drop cases: %w", err)
	}
	purge := istToday().AddDate(1, 0, 0)
	if err := q.SetCustomerCasesDocumentsPurgeAfter(txCtx, customersdb.SetCustomerCasesDocumentsPurgeAfterParams{
		CustomerID: customerID, PurgeAfter: pgtype.Date{Time: purge, Valid: true},
	}); err != nil {
		return Customer{}, fmt.Errorf("customers: purge dates: %w", err)
	}

	actorID, id := actor.ID, customerID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "customer.dropped", EntityType: "customer", EntityID: &id,
		Details: map[string]any{"reason_id": reason.ID, "reason": reason.Value},
	}); err != nil {
		return Customer{}, fmt.Errorf("customers: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Customer{}, fmt.Errorf("customers: commit: %w", err)
	}
	row, err := s.q.GetCustomer(ctx, customerID)
	if err != nil {
		return Customer{}, fmt.Errorf("customers: reload: %w", err)
	}
	return toCustomer(row.ID, row.Name, row.Mobile, row.Email, row.Status), nil
}

// istToday returns today's IST calendar date at midnight UTC.
func istToday() time.Time {
	local := time.Now().In(istZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
