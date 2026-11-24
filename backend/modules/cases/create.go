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
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	ErrInvalidStart    = errors.New("cases: start at Received or Logged in")
	ErrBankRequired    = errors.New("cases: choose a bank")
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
	LeadID         *int64
	UnitNumber     *string
}

// toCase maps a stored case row to the Case the API presents.
func toCase(row casesdb.GetCaseRow) Case {
	c := Case{
		ID: row.ID, CustomerID: row.CustomerID, BankID: row.BankID, ProjectID: row.ProjectID,
		LoanAmount: numericString(row.LoanAmount), TrackingNumber: textPtr(row.TrackingNumber),
		CurrentStageID: row.CurrentStageID, AssigneeID: row.AssigneeID, Status: row.Status,
		CreatedAt: row.CreatedAt.Time, UnitNumber: textPtr(row.UnitNumber),
	}
	if row.LeadID.Valid {
		id := row.LeadID.Int64
		c.LeadID = &id
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
	// UnitNumber is the flat, villa or plot number (optional).
	UnitNumber string
	// StartAt is "received" (default) or "logged_in"; logged_in needs TrackingNumber.
	StartAt        string
	TrackingNumber string
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

// Create opens a case for a customer at the bank's Received stage (or Logged
// in, with a tracking number), assigned to the given employee. It writes the
// stage and assignment history rows.
func (s *Service) Create(ctx context.Context, actor access.User, in NewCase) (Case, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Case{}, access.ErrForbidden
	}
	p, err := s.prepare(ctx, in)
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

	id, err := s.insertTx(txCtx, q, actor, in, p, nil)
	if err != nil {
		return Case{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, id)
}

// prepared holds the checked values of a NewCase.
type prepared struct {
	amount   pgtype.Numeric
	stages   []int64 // stage IDs reached at creation, in order; the last is current
	tracking pgtype.Text
	unit     pgtype.Text
}

// prepare checks everything about a new case that needs no lock.
func (s *Service) prepare(ctx context.Context, in NewCase) (prepared, error) {
	var p prepared
	var err error
	if p.amount, err = parseAmount(in.LoanAmount); err != nil {
		return p, err
	}
	if err := s.checkProject(ctx, in.ProjectID); err != nil {
		return p, err
	}
	if in.BankID <= 0 {
		return p, ErrBankRequired
	}
	if ok, err := s.q.BankExists(ctx, in.BankID); err != nil {
		return p, fmt.Errorf("cases: bank: %w", err)
	} else if !ok {
		return p, ErrBankRequired
	}
	if active, err := s.q.GetUserActive(ctx, in.AssigneeID); errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return p, ErrAssigneeInactive
	} else if err != nil {
		return p, fmt.Errorf("cases: assignee: %w", err)
	}
	unit, err := NormalizeUnit(in.UnitNumber)
	if err != nil {
		return p, err
	}
	p.unit = pgtype.Text{String: unit, Valid: unit != ""}
	start := in.StartAt
	if start == "" {
		start = startStageCode
	}
	if start != startStageCode && start != stageLoggedIn {
		return p, ErrInvalidStart
	}
	if start == stageLoggedIn {
		tr, err := NormalizeTracking(in.TrackingNumber)
		if err != nil {
			return p, err
		}
		if tr == "" {
			return p, ErrTrackingRequired
		}
		p.tracking = pgtype.Text{String: tr, Valid: true}
	}
	stages, err := s.workflows.StagesFor(ctx, in.BankID)
	if err != nil {
		return p, err
	}
	ids := map[string]int64{}
	for _, st := range stages {
		ids[st.Code] = st.ID
	}
	received, ok := ids[startStageCode]
	if !ok {
		return p, ErrNoStartStage
	}
	p.stages = []int64{received}
	if start == stageLoggedIn {
		logged, ok := ids[stageLoggedIn]
		if !ok {
			return p, ErrInvalidStart
		}
		p.stages = append(p.stages, logged)
	}
	return p, nil
}

// insertTx writes the case, its stage and assignment history, and the
// case.created audit entry inside the caller's transaction.
func (s *Service) insertTx(ctx context.Context, q *casesdb.Queries, actor access.User, in NewCase, p prepared, leadID *int64) (int64, error) {
	id, err := q.InsertCase(ctx, casesdb.InsertCaseParams{
		CustomerID:     in.CustomerID,
		BankID:         in.BankID,
		LoanAmount:     p.amount,
		CurrentStageID: p.stages[len(p.stages)-1],
		AssigneeID:     in.AssigneeID,
		CreatedBy:      pgtype.Int8{Int64: actor.ID, Valid: true},
		ProjectID:      in.ProjectID,
		TrackingNumber: p.tracking,
		LeadID:         int64Null(leadID),
		UnitNumber:     p.unit,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "cases_lead_id_key" {
			return 0, leads.ErrLeadClosed
		}
		if isUniqueViolation(err) && p.tracking.Valid {
			return 0, ErrDuplicateTracking
		}
		return 0, fmt.Errorf("cases: insert: %w", err)
	}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	for _, stageID := range p.stages {
		if err := q.InsertCaseStageEvent(ctx, casesdb.InsertCaseStageEventParams{
			CaseID: id, StageID: stageID, ReachedAt: now, ChangedBy: actor.ID,
		}); err != nil {
			return 0, fmt.Errorf("cases: stage event: %w", err)
		}
	}
	if err := q.InsertCaseAssignmentEvent(ctx, casesdb.InsertCaseAssignmentEventParams{
		CaseID: id, FromUserID: pgtype.Int8{}, ToUserID: in.AssigneeID, ChangedBy: actor.ID, ChangedAt: now,
	}); err != nil {
		return 0, fmt.Errorf("cases: assignment event: %w", err)
	}
	actorID, caseID := actor.ID, id
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID, Action: "case.created", EntityType: "case", EntityID: &caseID,
	}); err != nil {
		return 0, fmt.Errorf("cases: audit: %w", err)
	}
	return id, nil
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
