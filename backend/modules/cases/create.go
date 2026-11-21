// Package cases holds loan cases: creation, stage moves, assignment, drops,
// search and lists. A customer can have many cases (spec §4.2).
package cases

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the cases service.
var (
	ErrInvalidAmount = errors.New("cases: loan amount must be a number greater than zero")
	ErrNoStartStage  = errors.New("cases: bank has no Received stage")
	// ErrProjectRequired: every case belongs to a project (spec 2026-11-21 §4.3).
	ErrProjectRequired = errors.New("cases: choose a project, or NA")
	ErrProjectNotFound = errors.New("cases: project not found")
	// ErrCustomerDropped: a new case for a dropped customer needs BringBack.
	ErrCustomerDropped = errors.New("cases: this customer was dropped. Bring them back first")
)

// startStageCode is the stage every new case starts at (spec §5).
const startStageCode = "received"

// Case is one loan application for a customer.
type Case struct {
	ID             int64
	CustomerID     int64
	BankID         int64
	ProjectID      int64
	LoanAmount     string
	TrackingNumber *string
	CurrentStageID int64
	AssigneeID     int64
	Status         string
	CreatedAt      time.Time
	FollowUpDate   *time.Time
}

// toCase maps a stored case row to the Case the API presents.
func toCase(row casesdb.GetCaseRow) Case {
	c := Case{
		ID: row.ID, CustomerID: row.CustomerID, BankID: row.BankID, ProjectID: row.ProjectID,
		LoanAmount: numericString(row.LoanAmount), TrackingNumber: textPtr(row.TrackingNumber),
		CurrentStageID: row.CurrentStageID, AssigneeID: row.AssigneeID, Status: row.Status,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.FollowUpDate.Valid {
		d := row.FollowUpDate.Time
		c.FollowUpDate = &d
	}
	return c
}

// NewCase is the input for creating a case.
type NewCase struct {
	CustomerID int64
	BankID     int64
	ProjectID  int64
	LoanAmount string
	AssigneeID int64
	// BringBack makes a dropped customer active again in the same
	// transaction (spec 2026-11-21 §4.6). It does nothing for an active one.
	BringBack bool
}

// Service runs case actions. Each write is one transaction, including its
// history rows and audit entry.
type Service struct {
	pool      *pgxpool.Pool
	q         *casesdb.Queries
	workflows *workflows.Service
	rec       history.Recorder
	notifier  Notifier
}

// NewService returns a cases service on pool.
func NewService(pool *pgxpool.Pool, wf *workflows.Service, rec history.Recorder) *Service {
	return &Service{pool: pool, q: casesdb.New(pool), workflows: wf, rec: rec, notifier: nopNotifier{}}
}

// Create opens a case for a customer at the bank's Received stage, assigned
// to the given employee. It writes the first stage and assignment history rows.
func (s *Service) Create(ctx context.Context, actor access.User, in NewCase) (Case, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Case{}, access.ErrForbidden
	}
	amount, err := parseAmount(in.LoanAmount)
	if err != nil {
		return Case{}, err
	}
	if err := s.checkProject(ctx, in.ProjectID); err != nil {
		return Case{}, err
	}
	stageID, err := s.startStage(ctx, in.BankID)
	if err != nil {
		return Case{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("cases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	status, err := q.LockCustomerStatus(txCtx, in.CustomerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, customers.ErrCustomerNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: lock customer: %w", err)
	}
	if status == "dropped" {
		if !in.BringBack {
			return Case{}, ErrCustomerDropped
		}
		if err := customers.ReactivateTx(ctx, tx, s.rec, actor, in.CustomerID); err != nil {
			return Case{}, err
		}
	}

	row, err := q.InsertCase(txCtx, casesdb.InsertCaseParams{
		CustomerID:     in.CustomerID,
		BankID:         in.BankID,
		LoanAmount:     amount,
		CurrentStageID: stageID,
		AssigneeID:     in.AssigneeID,
		CreatedBy:      pgtype.Int8{Int64: actor.ID, Valid: true},
		ProjectID:      in.ProjectID,
	})
	if err != nil {
		return Case{}, fmt.Errorf("cases: insert: %w", err)
	}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if err := q.InsertCaseStageEvent(txCtx, casesdb.InsertCaseStageEventParams{
		CaseID: row.ID, StageID: stageID, ReachedAt: now, ChangedBy: actor.ID,
	}); err != nil {
		return Case{}, fmt.Errorf("cases: stage event: %w", err)
	}
	if err := q.InsertCaseAssignmentEvent(txCtx, casesdb.InsertCaseAssignmentEventParams{
		CaseID: row.ID, FromUserID: pgtype.Int8{}, ToUserID: in.AssigneeID, ChangedBy: actor.ID, ChangedAt: now,
	}); err != nil {
		return Case{}, fmt.Errorf("cases: assignment event: %w", err)
	}
	actorID, caseID := actor.ID, row.ID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "case.created", EntityType: "case", EntityID: &caseID,
	}); err != nil {
		return Case{}, fmt.Errorf("cases: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}

	return Case{
		ID:             row.ID,
		CustomerID:     row.CustomerID,
		BankID:         row.BankID,
		ProjectID:      row.ProjectID,
		LoanAmount:     in.LoanAmount,
		CurrentStageID: row.CurrentStageID,
		AssigneeID:     row.AssigneeID,
		Status:         row.Status,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

// checkProject refuses a missing or unknown project.
func (s *Service) checkProject(ctx context.Context, projectID int64) error {
	if projectID <= 0 {
		return ErrProjectRequired
	}
	ok, err := s.workflows.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrProjectNotFound
	}
	return nil
}

func (s *Service) startStage(ctx context.Context, bankID int64) (int64, error) {
	stages, err := s.workflows.StagesFor(ctx, bankID)
	if err != nil {
		return 0, err
	}
	for _, st := range stages {
		if st.Code == startStageCode {
			return st.ID, nil
		}
	}
	return 0, ErrNoStartStage
}

// parseAmount checks a loan amount typed by staff: a finite number above zero.
func parseAmount(raw string) (pgtype.Numeric, error) {
	s := strings.TrimSpace(raw)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return pgtype.Numeric{}, ErrInvalidAmount
	}
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, ErrInvalidAmount
	}
	return n, nil
}
