package customers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers/customersdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNotDropped is returned when bringing back a customer who is active.
var ErrNotDropped = errors.New("customers: customer is not dropped")

// ReactivateTx brings a dropped customer back to active inside tx: it clears
// the drop reason, remark, date and who dropped them, and audits
// customer.reactivated with the previous values. The customer's dropped
// cases stay dropped (spec 2026-11-21 §4.6). Callers check permission.
func ReactivateTx(ctx context.Context, tx pgx.Tx, rec history.Recorder, actor access.User, customerID int64) error {
	q := customersdb.New(tx)
	txCtx := history.WithTx(ctx, tx)
	cur, err := q.LockCustomerDrop(txCtx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCustomerNotFound
	}
	if err != nil {
		return fmt.Errorf("customers: lock: %w", err)
	}
	if cur.Status != "dropped" {
		return ErrNotDropped
	}
	if err := q.ReactivateCustomer(txCtx, customersdb.ReactivateCustomerParams{
		ID: customerID, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return fmt.Errorf("customers: reactivate: %w", err)
	}
	prev := map[string]any{"dropped_reason_id": nil, "dropped_remark": nil, "dropped_at": nil, "dropped_by": nil}
	if cur.DroppedReasonID.Valid {
		prev["dropped_reason_id"] = cur.DroppedReasonID.Int64
	}
	if cur.DroppedRemark.Valid {
		prev["dropped_remark"] = cur.DroppedRemark.String
	}
	if cur.DroppedAt.Valid {
		prev["dropped_at"] = cur.DroppedAt.Time
	}
	if cur.DroppedBy.Valid {
		prev["dropped_by"] = cur.DroppedBy.Int64
	}
	actorID, id := actor.ID, customerID
	if err := rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "customer.reactivated", EntityType: "customer", EntityID: &id, Details: prev,
	}); err != nil {
		return fmt.Errorf("customers: audit: %w", err)
	}
	return nil
}

// Reactivate brings a dropped customer back to active. All staff may (same
// permission as adding a record).
func (s *Service) Reactivate(ctx context.Context, actor access.User, customerID int64) (Customer, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Customer{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Customer{}, fmt.Errorf("customers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := ReactivateTx(ctx, tx, s.rec, actor, customerID); err != nil {
		return Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Customer{}, fmt.Errorf("customers: commit: %w", err)
	}
	return s.Get(ctx, customerID)
}
