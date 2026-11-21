package main

import (
	"context"
	"log"
	"time"
)

// ist is India Standard Time. Daily jobs run at 02:00 IST (plan Task 32).
var ist = time.FixedZone("IST", 5*60*60+30*60)

// schedule runs the daily jobs at 02:00 IST and the alert sender every 15
// minutes. It returns when ctx is done. Each job runs once per period.
func schedule(ctx context.Context, jobs Jobs, clock func() time.Time) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastDaily string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := clock()
		local := now.In(ist)
		if local.Hour() == 2 && local.Minute() == 0 {
			day := local.Format("2006-01-02")
			if day != lastDaily {
				lastDaily = day
				runJob(ctx, jobs, JobPurge, now)
				runJob(ctx, jobs, JobFollowUps, now)
			}
		}
		if local.Minute()%15 == 0 {
			runJob(ctx, jobs, JobAlertsSend, now)
		}
	}
}

func runJob(ctx context.Context, jobs Jobs, name string, now time.Time) {
	if err := jobs.Run(ctx, name, now); err != nil {
		log.Printf("job %s: %v", name, err)
	}
}
