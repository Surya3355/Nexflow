package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ist is India Standard Time. Daily jobs run at 02:00 IST (plan Task 32).
var ist = time.FixedZone("IST", 5*60*60+30*60)

// dailyJobs is the job_runs name for the purge and follow-up jobs, which run together.
const dailyJobs = "daily"

// JobRuns remembers the last day the daily jobs ran, so a restart neither
// skips nor repeats them (spec 2026-11-27-hardening §6).
type JobRuns interface {
	LastDay(ctx context.Context, name string) (time.Time, bool, error)
	SetDay(ctx context.Context, name string, day time.Time) error
}

// runFunc runs one named job.
type runFunc func(ctx context.Context, name string, now time.Time) error

type scheduler struct {
	run  runFunc
	runs JobRuns
}

func newScheduler(run runFunc, runs JobRuns) *scheduler {
	return &scheduler{run: run, runs: runs}
}

// tick runs what is due at now: the daily jobs once per IST day from 02:00
// (straight away when the server starts later than that and they haven't
// run today), and the alert sender every 15 minutes.
func (s *scheduler) tick(ctx context.Context, now time.Time) {
	local := now.In(ist)
	if local.Hour() >= 2 {
		today := dateOf(local)
		last, ok, err := s.runs.LastDay(ctx, dailyJobs)
		switch {
		case err != nil:
			log.Printf("job runs: %v", err)
		case !ok || last.Before(today):
			s.runOne(ctx, JobPurge, now)
			s.runOne(ctx, JobFollowUps, now)
			// Marked done even if a job failed: it is logged and tried again
			// tomorrow, rather than every minute.
			if err := s.runs.SetDay(ctx, dailyJobs, today); err != nil {
				log.Printf("job runs: %v", err)
			}
		}
	}
	if local.Minute()%15 == 0 {
		s.runOne(ctx, JobAlertsSend, now)
	}
}

func (s *scheduler) runOne(ctx context.Context, name string, now time.Time) {
	if err := s.run(ctx, name, now); err != nil {
		log.Printf("job %s: %v", name, err)
	}
}

// schedule ticks once at start and then every minute until ctx is done. A
// job in progress when ctx ends is allowed to finish (it runs without ctx's
// cancel), so a restart doesn't cut it off.
func schedule(ctx context.Context, s *scheduler, clock func() time.Time) {
	work := context.WithoutCancel(ctx)
	s.tick(work, clock())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(work, clock())
		}
	}
}

// dateOf is the calendar date of t in its own zone, as midnight UTC.
func dateOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// pgJobRuns stores job run days in the job_runs table.
type pgJobRuns struct{ pool *pgxpool.Pool }

func (p pgJobRuns) LastDay(ctx context.Context, name string) (time.Time, bool, error) {
	var d time.Time
	err := p.pool.QueryRow(ctx, `SELECT last_day FROM job_runs WHERE name = $1`, name).Scan(&d)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("job runs: read: %w", err)
	}
	return d, true, nil
}

func (p pgJobRuns) SetDay(ctx context.Context, name string, day time.Time) error {
	if _, err := p.pool.Exec(ctx, `INSERT INTO job_runs (name, last_day) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET last_day = EXCLUDED.last_day`, name, day); err != nil {
		return fmt.Errorf("job runs: save: %w", err)
	}
	return nil
}
