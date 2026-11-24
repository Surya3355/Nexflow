// Package leads holds enquiries for a builder project before the customer
// comes in: the lead record, its contact log, drop and bring back
// (spec 2026-11-23). Turning a lead into a case lives in package cases.
// This package must not import cases.
package leads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the leads service.
var (
	ErrLeadNotFound    = errors.New("leads: lead not found")
	ErrLeadClosed      = errors.New("leads: this lead is closed")
	ErrProjectRequired = errors.New("leads: choose a project, or NA")
	ErrProjectNotFound = errors.New("leads: project not found")
	ErrAssigneeInvalid = errors.New("leads: choose an active staff member")
	ErrLeadExists      = errors.New("leads: this customer already has an open lead for this project")
	ErrCustomerDropped = errors.New("leads: this customer was dropped. Bring them back first")
)

// Lead statuses.
const (
	StatusOpen      = "open"
	StatusConverted = "converted"
	StatusDropped   = "dropped"
)

const openLeadIndex = "leads_one_open_per_project_uidx"

// IST is India Standard Time. Call dates are IST days.
var IST = time.FixedZone("IST", 5*60*60+30*60)

// ISTDay returns the IST calendar day of t, as midnight UTC (the way pgtype.Date reads).
func ISTDay(t time.Time) time.Time {
	l := t.In(IST)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// Lead is one enquiry for one project.
type Lead struct {
	ID           int64
	CustomerID   int64
	ProjectID    int64
	AssigneeID   int64
	Status       string
	NextCallDate *time.Time
	CreatedAt    time.Time
	ConvertedAt  *time.Time
	CaseID       *int64
}

// Row is a lead with the names shown in lists, its last contact, and whether
// its call is due on the IST day of now.
type Row struct {
	Lead
	CustomerName   string
	CustomerMobile string
	CustomerEmail  string
	ProjectName    string
	AssigneeName   string
	LastResponse   string
	LastContactAt  *time.Time
	CallDue        bool
	OverdueDays    int
}

// Notifier queues the assignment alert for a lead's new assignee, inside the
// assignment transaction.
type Notifier interface {
	QueueLeadAssigned(ctx context.Context, leadID, recipientID int64) error
}

type nopNotifier struct{}

func (nopNotifier) QueueLeadAssigned(context.Context, int64, int64) error { return nil }

// Service runs lead actions. Each write is one transaction with its audit entry.
type Service struct {
	pool     *pgxpool.Pool
	q        *leadsdb.Queries
	rec      history.Recorder
	notifier Notifier
}

// NewService returns a leads service on pool.
func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service {
	return &Service{pool: pool, q: leadsdb.New(pool), rec: rec, notifier: nopNotifier{}}
}

// SetNotifier sets the assignment notifier. Without it, no alerts are queued.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

func (s *Service) audit(ctx context.Context, actor access.User, action string, leadID int64, details map[string]any) error {
	actorID, id := actor.ID, leadID
	if err := s.rec.Record(ctx, history.Entry{ActorUserID: &actorID, Action: action, EntityType: "lead", EntityID: &id, Details: details}); err != nil {
		return fmt.Errorf("leads: audit: %w", err)
	}
	return nil
}

// checkRefs validates the project and the assignee before any write.
func (s *Service) checkRefs(ctx context.Context, q *leadsdb.Queries, projectID, assigneeID int64) error {
	if projectID == 0 {
		return ErrProjectRequired
	}
	ok, err := q.ProjectExists(ctx, projectID)
	if err != nil {
		return fmt.Errorf("leads: project: %w", err)
	}
	if !ok {
		return ErrProjectNotFound
	}
	return s.checkAssignee(ctx, q, assigneeID)
}

func (s *Service) checkAssignee(ctx context.Context, q *leadsdb.Queries, assigneeID int64) error {
	ok, err := q.ActiveUserExists(ctx, assigneeID)
	if err != nil {
		return fmt.Errorf("leads: assignee: %w", err)
	}
	if !ok {
		return ErrAssigneeInvalid
	}
	return nil
}

// lockOpen locks a lead row and refuses unless it is open.
func lockOpen(ctx context.Context, q *leadsdb.Queries, leadID int64) (leadsdb.LockLeadRow, error) {
	row, err := q.LockLead(ctx, leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrLeadNotFound
	}
	if err != nil {
		return row, fmt.Errorf("leads: lock: %w", err)
	}
	if row.Status != StatusOpen {
		return row, ErrLeadClosed
	}
	return row, nil
}

func isOpenLeadConflict(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == openLeadIndex
}

// Assign moves an open lead to another active staff member and queues the
// assignment alert.
func (s *Service) Assign(ctx context.Context, actor access.User, leadID, toUserID int64) (Lead, error) {
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
	if _, err := lockOpen(txCtx, q, leadID); err != nil {
		return Lead{}, err
	}
	if err := s.checkAssignee(txCtx, q, toUserID); err != nil {
		return Lead{}, err
	}
	if err := q.SetLeadAssignee(txCtx, leadsdb.SetLeadAssigneeParams{ID: leadID, AssigneeID: toUserID, UpdatedBy: userRef(actor)}); err != nil {
		return Lead{}, fmt.Errorf("leads: assign: %w", err)
	}
	if err := s.notifier.QueueLeadAssigned(txCtx, leadID, toUserID); err != nil {
		return Lead{}, err
	}
	if err := s.audit(txCtx, actor, "lead.assigned", leadID, map[string]any{"assignee_id": toUserID}); err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}

func (s *Service) lead(ctx context.Context, id int64) (Lead, error) {
	r, err := s.Get(ctx, id, time.Now())
	return r.Lead, err
}

// queueDueToday queues the call-due reminder when the lead's call is today.
func queueDueToday(ctx context.Context, q *leadsdb.Queries, leadID int64, now time.Time) error {
	if err := q.QueueLeadDueToday(ctx, leadsdb.QueueLeadDueTodayParams{ID: leadID, Today: dateRef(ISTDay(now))}); err != nil {
		return fmt.Errorf("leads: queue reminder: %w", err)
	}
	return nil
}

func userRef(actor access.User) pgtype.Int8 { return pgtype.Int8{Int64: actor.ID, Valid: true} }

func dateRef(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }
