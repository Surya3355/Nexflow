package cases

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FromLead is the Add case form on a lead: the customer and project come
// from the lead (spec 2026-11-23 §4.5).
type FromLead struct {
	BankID         int64
	LoanAmount     string
	AssigneeID     int64
	UnitNumber     string
	StartAt        string
	TrackingNumber string
	// BuilderContactID: see NewCase.
	BuilderContactID int64
}

// CreateFromLead opens a case from an open lead and marks the lead converted,
// in one transaction. The lead row is locked first, so two people pressing
// Add case at once get one case; the second sees leads.ErrLeadClosed.
func (s *Service) CreateFromLead(ctx context.Context, actor access.User, leadID int64, in FromLead) (Case, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Case{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("cases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	lead, err := q.LockLeadForCase(txCtx, leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, leads.ErrLeadNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: lock lead: %w", err)
	}
	if lead.Status != leads.StatusOpen {
		return Case{}, leads.ErrLeadClosed
	}
	status, err := q.LockCustomerStatus(txCtx, lead.CustomerID)
	if err != nil {
		return Case{}, fmt.Errorf("cases: lock customer: %w", err)
	}
	if status == "dropped" {
		return Case{}, ErrCustomerDropped
	}

	nc := NewCase{
		CustomerID: lead.CustomerID, BankID: in.BankID, ProjectID: lead.ProjectID, LoanAmount: in.LoanAmount,
		AssigneeID: in.AssigneeID, UnitNumber: in.UnitNumber, StartAt: in.StartAt, TrackingNumber: in.TrackingNumber,
		BuilderContactID: in.BuilderContactID,
	}
	p, err := s.prepare(txCtx, nc)
	if err != nil {
		return Case{}, err
	}
	id, err := s.insertTx(txCtx, q, actor, nc, p, &leadID)
	if err != nil {
		return Case{}, err
	}
	if err := q.MarkLeadConverted(txCtx, casesdb.MarkLeadConvertedParams{ID: leadID, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true}}); err != nil {
		return Case{}, fmt.Errorf("cases: convert lead: %w", err)
	}
	actorID, lid := actor.ID, leadID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "lead.converted", EntityType: "lead", EntityID: &lid,
		Details: map[string]any{"case_id": id},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, id)
}
