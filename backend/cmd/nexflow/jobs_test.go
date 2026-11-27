package main

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/alerts"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/history"
)

type okSender struct{}

func (okSender) Send(context.Context, string, string, string) error { return nil }

func TestEachJobRunsOnce(t *testing.T) {
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	jobs := Jobs{
		Archive: archive.NewService(pool, history.Nop{}),
		Queue:   alerts.NewQueue(pool),
		Sender:  okSender{},
	}
	now := time.Now()
	for _, name := range []string{JobPurge, JobFollowUps, JobAlertsSend} {
		if err := jobs.Run(context.Background(), name, now); err != nil {
			t.Fatalf("job %s: %v", name, err)
		}
	}
}

func TestUnknownJobRefused(t *testing.T) {
	jobs := Jobs{}
	if err := jobs.Run(context.Background(), "backup-everything", time.Now()); err == nil {
		t.Fatal("unknown job was accepted")
	}
}

// Until SMTP is configured, alerts are not marked as sent.
func TestUnconfiguredSenderLeavesAlertsQueued(t *testing.T) {
	if err := (unconfiguredSender{}).Send(context.Background(), "a@b", "s", "b"); err == nil {
		t.Fatal("unconfigured sender reported success")
	}
}
