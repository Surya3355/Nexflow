// Package alerts queues and sends email alerts (spec §10): case assignment,
// and follow-up due or overdue. Alerts are queued in the same transaction as
// the change, then sent by a job. A failed send is kept for retry.
package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/alerts/alertsdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Alert kinds (database-schema §7).
const (
	KindAssigned        = "assigned"
	KindFollowUpDue     = "follow_up_due"
	KindFollowUpOverdue = "follow_up_overdue"
	KindQueryDue        = "query_due"
	KindQueryOverdue    = "query_overdue"
)

// Sender sends one email. The SMTP implementation waits for hosting (plan Task 34 note).
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// DispatchResult counts one send run.
type DispatchResult struct {
	Sent   int
	Failed int
}

// Queue writes alerts and sends them.
type Queue struct {
	pool *pgxpool.Pool
}

// NewQueue returns a queue on pool.
func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool}
}

// executor returns the open transaction in ctx, or the pool.
func (q *Queue) executor(ctx context.Context) alertsdb.DBTX {
	if tx, ok := history.TxFromContext(ctx); ok {
		return tx
	}
	return q.pool
}

// Enqueue adds an alert. A duplicate (same case, kind, due date and recipient)
// is ignored, so each alert is queued once (database-schema §7).
func (q *Queue) Enqueue(ctx context.Context, caseID, recipientID int64, kind string, dueOn *time.Time) error {
	var due pgtype.Date
	if dueOn != nil {
		due = pgtype.Date{Time: *dueOn, Valid: true}
	}
	if _, err := alertsdb.New(q.executor(ctx)).EnqueueAlert(ctx, alertsdb.EnqueueAlertParams{
		CaseID: pgtype.Int8{Int64: caseID, Valid: true}, RecipientUserID: recipientID, Kind: kind, DueOn: due,
	}); err != nil {
		return fmt.Errorf("alerts: enqueue: %w", err)
	}
	return nil
}

// QueueAssigned queues the assignment alert for the new assignee. It satisfies
// cases.Notifier, and runs inside the assignment transaction.
func (q *Queue) QueueAssigned(ctx context.Context, caseID, recipientID int64) error {
	return q.Enqueue(ctx, caseID, recipientID, KindAssigned, nil)
}

// QueueLeadAssigned queues the assignment alert for a lead's new assignee. It
// satisfies leads.Notifier, and runs inside the assignment transaction.
func (q *Queue) QueueLeadAssigned(ctx context.Context, leadID, recipientID int64) error {
	if _, err := alertsdb.New(q.executor(ctx)).EnqueueLeadAlert(ctx, alertsdb.EnqueueLeadAlertParams{
		LeadID: pgtype.Int8{Int64: leadID, Valid: true}, RecipientUserID: recipientID, Kind: KindAssigned,
	}); err != nil {
		return fmt.Errorf("alerts: enqueue lead: %w", err)
	}
	return nil
}

// Dispatch sends up to limit queued alerts, those tried least first, so
// failing alerts never hold back newer ones. Alerts older than 3 days, for a
// person with no email address, or failed 5 times are skipped, not sent.
func (q *Queue) Dispatch(ctx context.Context, sender Sender, limit int32) (DispatchResult, error) {
	db := alertsdb.New(q.pool)
	if _, err := db.SkipStaleAlerts(ctx); err != nil {
		return DispatchResult{}, fmt.Errorf("alerts: skip stale: %w", err)
	}
	rows, err := db.UnsentAlerts(ctx, limit)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("alerts: unsent: %w", err)
	}
	var res DispatchResult
	for _, r := range rows {
		if r.Email == "" {
			res.Failed++
			if err := db.SkipAlert(ctx, alertsdb.SkipAlertParams{ID: r.ID, SkipReason: pgtype.Text{String: "no email address", Valid: true}}); err != nil {
				return res, fmt.Errorf("alerts: skip: %w", err)
			}
			continue
		}
		subject, body := compose(r.Kind, r.CustomerName, r.ProjectName, r.CaseRef, r.LeadRef)
		if err := sender.Send(ctx, r.Email, subject, body); err != nil {
			res.Failed++
			if err := db.MarkAlertFailed(ctx, alertsdb.MarkAlertFailedParams{ID: r.ID, LastError: pgtype.Text{String: err.Error(), Valid: true}}); err != nil {
				return res, fmt.Errorf("alerts: mark failed: %w", err)
			}
			continue
		}
		if err := db.MarkAlertSent(ctx, r.ID); err != nil {
			return res, fmt.Errorf("alerts: mark sent: %w", err)
		}
		res.Sent++
	}
	return res, nil
}

func compose(kind, customer, project string, caseRef, leadRef pgtype.Int8) (string, string) {
	if leadRef.Valid && !caseRef.Valid {
		switch kind {
		case KindAssigned:
			return "A lead has been assigned to you", fmt.Sprintf("Lead for %s (%s) has been assigned to you in Nexflow.", customer, project)
		case KindFollowUpDue:
			return "Call due today", fmt.Sprintf("Call %s about %s today.", customer, project)
		default:
			return "Call overdue", fmt.Sprintf("The call to %s about %s is overdue.", customer, project)
		}
	}
	caseID := caseRef.Int64
	switch kind {
	case KindAssigned:
		return "A case has been assigned to you", fmt.Sprintf("Case %d for %s has been assigned to you in Nexflow.", caseID, customer)
	case KindFollowUpDue:
		return "Follow-up due today", fmt.Sprintf("Case %d for %s has a follow-up due today.", caseID, customer)
	case KindQueryDue:
		return "Bank query due today", fmt.Sprintf("Case %d for %s has a bank query due today.", caseID, customer)
	case KindQueryOverdue:
		return "Bank query overdue", fmt.Sprintf("Case %d for %s has a bank query that is overdue.", caseID, customer)
	default:
		return "Follow-up overdue", fmt.Sprintf("Case %d for %s has a follow-up that is overdue.", caseID, customer)
	}
}
