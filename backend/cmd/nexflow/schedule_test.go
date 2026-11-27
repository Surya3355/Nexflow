package main

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
)

// memRuns keeps the last run day in memory.
type memRuns struct{ days map[string]time.Time }

func (m *memRuns) LastDay(_ context.Context, name string) (time.Time, bool, error) {
	d, ok := m.days[name]
	return d, ok, nil
}

func (m *memRuns) SetDay(_ context.Context, name string, day time.Time) error {
	m.days[name] = day
	return nil
}

type recorded struct{ names []string }

func (r *recorded) run(_ context.Context, name string, _ time.Time) error {
	r.names = append(r.names, name)
	return nil
}

func (r *recorded) count(name string) int {
	n := 0
	for _, x := range r.names {
		if x == name {
			n++
		}
	}
	return n
}

func atIST(day, hhmm string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, ist)
	return t
}

func TestDailyCatchUpOnStart(t *testing.T) {
	runs := &memRuns{days: map[string]time.Time{dailyJobs: dateOf(atIST("2026-11-26", "02:00"))}}
	rec := &recorded{}
	s := newScheduler(rec.run, runs)
	// The server was down at 02:00 and starts at 09:07: today's daily jobs run now.
	s.tick(context.Background(), atIST("2026-11-27", "09:07"))
	if rec.count(JobPurge) != 1 || rec.count(JobFollowUps) != 1 {
		t.Fatalf("ran %v, want purge and followups once", rec.names)
	}
	if d, _, _ := runs.LastDay(context.Background(), dailyJobs); !d.Equal(dateOf(atIST("2026-11-27", "00:00"))) {
		t.Fatalf("last day = %v, want 2026-11-27", d)
	}
}

func TestDailyNotBefore2am(t *testing.T) {
	runs := &memRuns{days: map[string]time.Time{dailyJobs: dateOf(atIST("2026-11-26", "02:00"))}}
	rec := &recorded{}
	s := newScheduler(rec.run, runs)
	s.tick(context.Background(), atIST("2026-11-27", "01:31"))
	if rec.count(JobPurge) != 0 {
		t.Fatalf("daily jobs ran before 02:00: %v", rec.names)
	}
	s.tick(context.Background(), atIST("2026-11-27", "02:00"))
	if rec.count(JobPurge) != 1 {
		t.Fatalf("daily jobs did not run at 02:00: %v", rec.names)
	}
}

func TestDailyOncePerDay(t *testing.T) {
	runs := &memRuns{days: map[string]time.Time{}}
	rec := &recorded{}
	s := newScheduler(rec.run, runs)
	for _, hhmm := range []string{"02:00", "02:01", "13:45", "23:59"} {
		s.tick(context.Background(), atIST("2026-11-27", hhmm))
	}
	// A restart: a new scheduler reads the stored day and does not run again.
	newScheduler(rec.run, runs).tick(context.Background(), atIST("2026-11-27", "23:59"))
	if rec.count(JobPurge) != 1 || rec.count(JobFollowUps) != 1 {
		t.Fatalf("ran %v, want each daily job once", rec.names)
	}
}

func TestAlertsEveryQuarterHour(t *testing.T) {
	rec := &recorded{}
	s := newScheduler(rec.run, &memRuns{days: map[string]time.Time{dailyJobs: dateOf(atIST("2026-11-27", "00:00"))}})
	for _, hhmm := range []string{"10:00", "10:01", "10:14", "10:15", "10:30"} {
		s.tick(context.Background(), atIST("2026-11-27", hhmm))
	}
	if n := rec.count(JobAlertsSend); n != 3 {
		t.Fatalf("alert sends = %d, want 3 (10:00, 10:15, 10:30)", n)
	}
}

func TestPgJobRuns(t *testing.T) {
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatal(err)
	}
	r := pgJobRuns{pool: pool}
	ctx := context.Background()
	if _, ok, err := r.LastDay(ctx, dailyJobs); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	for _, d := range []string{"2026-11-26", "2026-11-27"} {
		day, _ := time.Parse(time.DateOnly, d)
		if err := r.SetDay(ctx, dailyJobs, day); err != nil {
			t.Fatal(err)
		}
	}
	got, ok, err := r.LastDay(ctx, dailyJobs)
	if err != nil || !ok || got.Format(time.DateOnly) != "2026-11-27" {
		t.Fatalf("LastDay = %v %v %v, want 2026-11-27", got, ok, err)
	}
}
