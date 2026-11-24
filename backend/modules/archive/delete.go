// Package archive handles hard delete and restore. A deleted customer or case
// is kept as a full JSON copy for one year, then purged (spec §4.5). Business
// rows are never physically deleted anywhere else.
package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive/archivedb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors for hard delete.
var (
	ErrCustomerHasCases = errors.New("archive: customer still has live cases; archive them first")
	// ErrCustomerHasOpenLeads: dropping the customer drops their open leads.
	ErrCustomerHasOpenLeads = errors.New("archive: this customer has open leads. Drop the customer first")
	ErrNotFound         = errors.New("archive: record not found")
)

// Service runs hard delete and restore.
type Service struct {
	pool  *pgxpool.Pool
	q     *archivedb.Queries
	rec   history.Recorder
	files FileRemover
}

// NewService returns an archive service on pool.
func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service {
	return &Service{pool: pool, q: archivedb.New(pool), rec: rec, files: nopRemover{}}
}

// DeleteCase archives a case with its customer's details, stage and assignment
// history, documents and alerts, then removes the live rows. Manager and admin only.
func (s *Service) DeleteCase(ctx context.Context, actor access.User, caseID int64) error {
	if !access.Can(actor.Role, access.ActionHardDelete) {
		return access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("archive: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	caseDoc, err := q.CaseRowJSON(txCtx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive: read case: %w", err)
	}
	var head struct {
		CustomerID int64 `json:"customer_id"`
	}
	if err := json.Unmarshal([]byte(caseDoc), &head); err != nil {
		return fmt.Errorf("archive: case json: %w", err)
	}
	customerDoc, err := q.CustomerRowJSON(txCtx, head.CustomerID)
	if err != nil {
		return fmt.Errorf("archive: read customer: %w", err)
	}
	children, err := q.CaseChildrenJSON(txCtx, caseID)
	if err != nil {
		return fmt.Errorf("archive: read history: %w", err)
	}

	snapshot, err := json.Marshal(map[string]json.RawMessage{
		"case":     json.RawMessage(caseDoc),
		"customer": json.RawMessage(customerDoc),
		"children": json.RawMessage(children),
	})
	if err != nil {
		return fmt.Errorf("archive: snapshot: %w", err)
	}

	if err := q.InsertArchive(txCtx, archivedb.InsertArchiveParams{
		EntityType: "case", EntityID: caseID, Snapshot: snapshot, ArchivedBy: actor.ID,
	}); err != nil {
		return fmt.Errorf("archive: insert: %w", err)
	}
	if err := q.DeleteCaseAlerts(txCtx, pgtype.Int8{Int64: caseID, Valid: true}); err != nil {
		return fmt.Errorf("archive: alerts: %w", err)
	}
	if err := q.DeleteCaseDocuments(txCtx, caseID); err != nil {
		return fmt.Errorf("archive: documents: %w", err)
	}
	if err := q.DeleteCaseStageEvents(txCtx, caseID); err != nil {
		return fmt.Errorf("archive: stage events: %w", err)
	}
	if err := q.DeleteCaseAssignmentEvents(txCtx, caseID); err != nil {
		return fmt.Errorf("archive: assignment events: %w", err)
	}
	if err := q.DeleteCaseRow(txCtx, caseID); err != nil {
		return fmt.Errorf("archive: delete case: %w", err)
	}
	if err := s.audit(txCtx, actor, "case.deleted", "case", caseID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteCustomer archives a customer that has no live cases. The archive entry
// lists the IDs of the cases archived for it (owner answer D).
func (s *Service) DeleteCustomer(ctx context.Context, actor access.User, customerID int64) error {
	if !access.Can(actor.Role, access.ActionHardDelete) {
		return access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("archive: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	if _, err := q.LockCustomerForDelete(txCtx, customerID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("archive: lock customer: %w", err)
	}
	customerDoc, err := q.CustomerRowJSON(txCtx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive: read customer: %w", err)
	}
	live, err := q.CountLiveCases(txCtx, customerID)
	if err != nil {
		return fmt.Errorf("archive: count cases: %w", err)
	}
	if live > 0 {
		return ErrCustomerHasCases
	}
	open, err := q.CountOpenLeads(txCtx, customerID)
	if err != nil {
		return fmt.Errorf("archive: count leads: %w", err)
	}
	if open > 0 {
		return ErrCustomerHasOpenLeads
	}
	leadsDoc, err := q.CustomerLeadsJSON(txCtx, customerID)
	if err != nil {
		return fmt.Errorf("archive: read leads: %w", err)
	}
	caseIDs, err := q.ArchivedCaseIDs(txCtx, customerID)
	if err != nil {
		return fmt.Errorf("archive: case ids: %w", err)
	}
	if caseIDs == nil {
		caseIDs = []int64{}
	}

	snapshot, err := json.Marshal(map[string]any{
		"customer": json.RawMessage(customerDoc),
		"case_ids": caseIDs,
		"leads":    json.RawMessage(leadsDoc),
	})
	if err != nil {
		return fmt.Errorf("archive: snapshot: %w", err)
	}
	if err := q.InsertArchive(txCtx, archivedb.InsertArchiveParams{
		EntityType: "customer", EntityID: customerID, Snapshot: snapshot, ArchivedBy: actor.ID,
	}); err != nil {
		return fmt.Errorf("archive: insert: %w", err)
	}
	if err := q.DeleteCustomerLeadAlerts(txCtx, customerID); err != nil {
		return fmt.Errorf("archive: delete lead alerts: %w", err)
	}
	if err := q.DeleteCustomerLeadContacts(txCtx, customerID); err != nil {
		return fmt.Errorf("archive: delete lead contacts: %w", err)
	}
	if err := q.DeleteCustomerLeads(txCtx, customerID); err != nil {
		return fmt.Errorf("archive: delete leads: %w", err)
	}
	if err := q.DeleteCustomerRow(txCtx, customerID); err != nil {
		return fmt.Errorf("archive: delete customer: %w", err)
	}
	if err := s.audit(txCtx, actor, "customer.deleted", "customer", customerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) audit(ctx context.Context, actor access.User, action, entityType string, id int64) error {
	actorID, entity := actor.ID, id
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID, Action: action, EntityType: entityType, EntityID: &entity,
	}); err != nil {
		return fmt.Errorf("archive: audit: %w", err)
	}
	return nil
}

// Item is one archive row as the admin archive screen shows it.
type Item struct {
	ID         int64
	EntityType string
	EntityID   int64
	ArchivedAt time.Time
	PurgeAfter time.Time
	Restored   bool
}

// List returns archive rows, newest first (admin screen, spec §4.5).
func (s *Service) List(ctx context.Context) ([]Item, error) {
	rows, err := s.q.ListArchive(ctx)
	if err != nil {
		return nil, fmt.Errorf("archive: list: %w", err)
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, Item{
			ID: r.ID, EntityType: r.EntityType, EntityID: r.EntityID,
			ArchivedAt: r.ArchivedAt.Time, PurgeAfter: r.PurgeAfter.Time, Restored: r.RestoredAt.Valid,
		})
	}
	return out, nil
}
