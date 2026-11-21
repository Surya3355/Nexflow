package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/alerts/alertsdb"
	"github.com/jackc/pgx/v5/pgtype"
)

var istZone = time.FixedZone("IST", 5*60*60+30*60)

// FollowUpResult counts the follow-up alerts queued by one run.
type FollowUpResult struct {
	Due     int
	Overdue int
}

// RunFollowUps queues follow-up alerts for the IST day of now (spec §10):
// a due alert on the follow-up date, and an overdue alert on the day after.
// Only active cases get them, and each alert is queued once (plan Task 30).
func (q *Queue) RunFollowUps(ctx context.Context, now time.Time) (FollowUpResult, error) {
	local := now.In(istZone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	db := alertsdb.New(q.pool)
	due, err := db.QueueFollowUpDue(ctx, pgtype.Date{Time: today, Valid: true})
	if err != nil {
		return FollowUpResult{}, fmt.Errorf("alerts: due: %w", err)
	}
	overdue, err := db.QueueFollowUpOverdue(ctx, pgtype.Date{Time: yesterday, Valid: true})
	if err != nil {
		return FollowUpResult{}, fmt.Errorf("alerts: overdue: %w", err)
	}
	return FollowUpResult{Due: int(due), Overdue: int(overdue)}, nil
}
