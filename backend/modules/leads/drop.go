package leads

import (
	"context"
	"errors"
	"fmt"
	"github.com/Surya3355/nexflow/backend/internal/platform/validate"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Drop errors, matching the case drop rules (spec §4.2).
var (
	ErrInvalidReason      = errors.New("leads: choose a valid drop reason")
	ErrDropRemarkRequired = errors.New("leads: a remark is required when the reason is Other")
	// ErrLeadHasCase: a dropped lead that a case still points at stays closed.
	ErrLeadHasCase = errors.New("leads: this lead already has a case")
)

const otherReason = "Other"

// Drop closes an open lead with a reason from the admin list.
func (s *Service) Drop(ctx context.Context, actor access.User, leadID, reasonID int64, remark string) (Lead, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Lead{}, access.ErrForbidden
	}
	if reasonID == 0 {
		return Lead{}, ErrInvalidReason
	}
	reason, err := s.q.GetDropReason(ctx, reasonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrInvalidReason
	}
	if err != nil {
		return Lead{}, fmt.Errorf("leads: reason: %w", err)
	}
	remark = strings.TrimSpace(remark)
	if reason.Value == otherReason && remark == "" {
		return Lead{}, ErrDropRemarkRequired
	}
	if _, err := validate.OptionalText(remark, validate.MaxText, "remark"); err != nil {
		return Lead{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)
	if _, err := lockOpen(txCtx, q, leadID); err != nil {
		return Lead{}, err
	}
	if err := q.DropLead(txCtx, leadsdb.DropLeadParams{
		ID: leadID, DroppedReasonID: pgtype.Int8{Int64: reason.ID, Valid: true},
		DroppedRemark: pgtype.Text{String: remark, Valid: remark != ""}, DroppedBy: userRef(actor),
	}); err != nil {
		return Lead{}, fmt.Errorf("leads: drop: %w", err)
	}
	if err := s.audit(txCtx, actor, "lead.dropped", leadID, map[string]any{"reason_id": reason.ID, "reason": reason.Value}); err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}

// BringBack reopens a dropped lead with a call due on the IST day of now,
// bringing the customer back too if they were dropped.
func (s *Service) BringBack(ctx context.Context, actor access.User, leadID int64, now time.Time) (Lead, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Lead{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	cur, err := q.LockLead(txCtx, leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrLeadNotFound
	}
	if err != nil {
		return Lead{}, fmt.Errorf("leads: lock: %w", err)
	}
	if cur.Status != StatusDropped {
		return Lead{}, ErrLeadClosed
	}
	if linked, err := q.LeadHasCase(txCtx, pgtype.Int8{Int64: leadID, Valid: true}); err != nil {
		return Lead{}, fmt.Errorf("leads: case check: %w", err)
	} else if linked {
		return Lead{}, ErrLeadHasCase
	}
	if err := ensureCustomerActive(ctx, tx, q, s.rec, actor, cur.CustomerID, true); err != nil {
		return Lead{}, err
	}
	err = q.ReopenLead(txCtx, leadsdb.ReopenLeadParams{ID: leadID, NextCallDate: dateRef(ISTDay(now)), UpdatedBy: userRef(actor)})
	if isOpenLeadConflict(err) {
		return Lead{}, ErrLeadExists
	}
	if err != nil {
		return Lead{}, fmt.Errorf("leads: reopen: %w", err)
	}
	if err := s.audit(txCtx, actor, "lead.brought_back", leadID, nil); err != nil {
		return Lead{}, err
	}
	if err := queueDueToday(txCtx, q, leadID, now); err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}
