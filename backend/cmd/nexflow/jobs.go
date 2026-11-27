package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/alerts"
	"github.com/Surya3355/nexflow/backend/modules/archive"
)

// Job names (plan Task 32).
const (
	JobPurge      = "purge"
	JobFollowUps  = "followups"
	JobAlertsSend = "alerts-send"
)

// alertBatch is how many queued alerts one send run handles.
const alertBatch = 100

// Jobs holds what the scheduled jobs need.
type Jobs struct {
	Archive *archive.Service
	Queue   *alerts.Queue
	Sender  alerts.Sender
}

// Run runs one job once. An unknown name is refused.
func (j Jobs) Run(ctx context.Context, name string, now time.Time) error {
	switch name {
	case JobPurge:
		_, err := j.Archive.PurgeExpired(ctx, now)
		return err
	case JobFollowUps:
		_, err := j.Queue.RunFollowUps(ctx, now)
		return err
	case JobAlertsSend:
		_, err := j.Queue.Dispatch(ctx, j.Sender, alertBatch)
		return err
	}
	return fmt.Errorf("jobs: unknown job %q", name)
}

// unconfiguredSender refuses every send. It is used until SMTP is set up, so
// alerts stay queued and are never marked as sent.
type unconfiguredSender struct{}

func (unconfiguredSender) Send(context.Context, string, string, string) error {
	return fmt.Errorf("email is not configured yet")
}
