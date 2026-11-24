package cases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrAssigneeInactive is returned when a case is given to a deactivated or unknown user.
var ErrAssigneeInactive = errors.New("cases: the person you are assigning to is not active")

// Notifier queues the alert for a new assignee. It runs inside the assignment
// transaction, so the alert is recorded only if the assignment is saved.
// The alerts module provides the implementation (spec §10).
type Notifier interface {
	QueueAssigned(ctx context.Context, caseID, recipientID int64) error
}

type nopNotifier struct{}

func (nopNotifier) QueueAssigned(context.Context, int64, int64) error { return nil }

// SetNotifier sets the assignment notifier. Without it, no alerts are queued.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// Assign gives a case to another staff member. Any staff member may assign at
// any stage (spec §6, round 8). Each change adds a history row. Assigning a
// case to the person who already has it changes nothing.
func (s *Service) Assign(ctx context.Context, actor access.User, caseID, toUserID int64) (Case, error) {
	if !access.Can(actor.Role, access.ActionAssignCase) {
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
	if locked.AssigneeID == toUserID {
		return s.load(ctx, caseID)
	}

	active, err := q.GetUserActive(txCtx, toUserID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return Case{}, ErrAssigneeInactive
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: check assignee: %w", err)
	}

	now := time.Now()
	if err := q.SetCaseAssignee(txCtx, casesdb.SetCaseAssigneeParams{
		ID: caseID, AssigneeID: toUserID, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: set assignee: %w", err)
	}
	if err := q.InsertCaseAssignmentEventAt(txCtx, casesdb.InsertCaseAssignmentEventAtParams{
		CaseID: caseID, FromUserID: pgtype.Int8{Int64: locked.AssigneeID, Valid: true},
		ToUserID: toUserID, ChangedBy: actor.ID, ChangedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: assignment event: %w", err)
	}

	actorID, id := actor.ID, caseID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.assigned", EntityType: "case", EntityID: &id,
		Details: map[string]any{"from_user_id": locked.AssigneeID, "to_user_id": toUserID},
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := s.notifier.QueueAssigned(txCtx, caseID, toUserID); err != nil {
		return Case{}, fmt.Errorf("cases: queue alert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

// Get reads a case by ID as the API presents it.
func (s *Service) Get(ctx context.Context, caseID int64) (Case, error) {
	return s.load(ctx, caseID)
}

// load reads a case by ID as the API presents it.
func (s *Service) load(ctx context.Context, caseID int64) (Case, error) {
	row, err := s.q.GetCase(ctx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: load: %w", err)
	}
	c := toCase(row)
	typ, err := s.q.ProjectType(ctx, c.ProjectID)
	if err != nil {
		return Case{}, fmt.Errorf("cases: load project type: %w", err)
	}
	if typ.Valid {
		v := typ.String
		c.ProjectType = &v
	}
	if c.BuilderContactID != nil {
		bc, err := s.q.GetBuilderContact(ctx, *c.BuilderContactID)
		if err != nil {
			return Case{}, fmt.Errorf("cases: load contact: %w", err)
		}
		c.BuilderContact = &BuilderContact{ID: bc.ID, Name: bc.Name, Phone: bc.Phone, Email: bc.Email, Label: bc.Label, Removed: bc.Removed}
	}
	return c, nil
}
