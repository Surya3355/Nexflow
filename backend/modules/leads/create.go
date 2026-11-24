package leads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// NewLead is the New customer form: the customer and their first lead.
type NewLead struct {
	Name, Mobile, Email   string
	ProjectID, AssigneeID int64
}

// CreateResult is the outcome of Create. When the mobile already belongs to a
// customer, Existing is set and nothing is created (round 14 §4.6).
type CreateResult struct {
	Lead     *Lead
	Customer *customers.Customer
	Existing *customers.Customer
}

// Create adds a customer and their first lead in one transaction. The lead's
// first call is due on the IST day of now.
func (s *Service) Create(ctx context.Context, actor access.User, in NewLead, now time.Time) (CreateResult, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return CreateResult{}, access.ErrForbidden
	}
	mobile, err := customers.NormalizeMobile(in.Mobile)
	if err != nil {
		return CreateResult{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CreateResult{}, customers.ErrNameRequired
	}
	if err := s.checkRefs(ctx, s.q, in.ProjectID, in.AssigneeID); err != nil {
		return CreateResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	email := strings.TrimSpace(in.Email)
	row, err := q.InsertCustomerForLead(txCtx, leadsdb.InsertCustomerForLeadParams{
		Name: name, Mobile: mobile, Email: pgtype.Text{String: email, Valid: email != ""}, CreatedBy: userRef(actor),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		ex, err := s.q.GetCustomerByMobile(ctx, mobile)
		if err != nil {
			return CreateResult{}, fmt.Errorf("leads: read existing: %w", err)
		}
		c := customers.Customer{ID: ex.ID, Name: ex.Name, Mobile: ex.Mobile, Email: ex.Email.String, Status: ex.Status}
		return CreateResult{Existing: &c}, nil
	}
	if err != nil {
		return CreateResult{}, fmt.Errorf("leads: insert customer: %w", err)
	}
	cust := customers.Customer{ID: row.ID, Name: row.Name, Mobile: row.Mobile, Email: row.Email.String, Status: row.Status}
	actorID, custID := actor.ID, cust.ID
	if err := s.rec.Record(txCtx, history.Entry{ActorUserID: &actorID, Action: "customer.created", EntityType: "customer", EntityID: &custID}); err != nil {
		return CreateResult{}, fmt.Errorf("leads: audit: %w", err)
	}
	leadID, err := s.insertLead(txCtx, q, actor, cust.ID, in.ProjectID, in.AssigneeID, now)
	if err != nil {
		return CreateResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("leads: commit: %w", err)
	}
	l, err := s.lead(ctx, leadID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Lead: &l, Customer: &cust}, nil
}

// AddForCustomer adds another lead (a new project) for an existing customer.
// A dropped customer needs bringBack, which makes them active in the same
// transaction; their old cases and leads stay dropped.
func (s *Service) AddForCustomer(ctx context.Context, actor access.User, customerID, projectID, assigneeID int64, bringBack bool, now time.Time) (Lead, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Lead{}, access.ErrForbidden
	}
	if err := s.checkRefs(ctx, s.q, projectID, assigneeID); err != nil {
		return Lead{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	if err := ensureCustomerActive(ctx, tx, q, s.rec, actor, customerID, bringBack); err != nil {
		return Lead{}, err
	}
	leadID, err := s.insertLead(txCtx, q, actor, customerID, projectID, assigneeID, now)
	if err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}

// ensureCustomerActive locks the customer and, if dropped, brings them back
// when allowed.
func ensureCustomerActive(ctx context.Context, tx pgx.Tx, q *leadsdb.Queries, rec history.Recorder, actor access.User, customerID int64, bringBack bool) error {
	status, err := q.LockCustomerStatus(history.WithTx(ctx, tx), customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return customers.ErrCustomerNotFound
	}
	if err != nil {
		return fmt.Errorf("leads: lock customer: %w", err)
	}
	if status != "dropped" {
		return nil
	}
	if !bringBack {
		return ErrCustomerDropped
	}
	return customers.ReactivateTx(ctx, tx, rec, actor, customerID)
}

func (s *Service) insertLead(ctx context.Context, q *leadsdb.Queries, actor access.User, customerID, projectID, assigneeID int64, now time.Time) (int64, error) {
	id, err := q.InsertLead(ctx, leadsdb.InsertLeadParams{
		CustomerID: customerID, ProjectID: projectID, AssigneeID: assigneeID,
		NextCallDate: dateRef(ISTDay(now)), CreatedBy: userRef(actor),
	})
	if isOpenLeadConflict(err) {
		return 0, ErrLeadExists
	}
	if err != nil {
		return 0, fmt.Errorf("leads: insert: %w", err)
	}
	if err := s.audit(ctx, actor, "lead.created", id, map[string]any{"customer_id": customerID, "project_id": projectID, "assignee_id": assigneeID}); err != nil {
		return 0, err
	}
	if err := queueDueToday(ctx, q, id, now); err != nil {
		return 0, err
	}
	return id, nil
}
