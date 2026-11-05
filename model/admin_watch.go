package model

import (
	"context"
	"log"
	"starhub/config"
	"time"
)

func WatchAdministration(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastDay := ""
	lastRun := time.Time{}
	lastPrune := time.Time{}
	for {
		profile, err := CurrentDeploymentProfile(ctx)
		if err == nil && profile.Revision > 0 {
			_ = config.SetRegionTimezone(profile.Profile.Timezone)
		}
		now := time.Now()
		loc, zoneErr := time.LoadLocation(config.RegionTimezone())
		if zoneErr == nil && now.Sub(lastRun) >= time.Minute {
			day := now.In(loc).Format("2006-01-02")
			if lastDay != day {
				if _, err := RunDailyReconciliation(ctx, now.In(loc).AddDate(0, 0, -1).Format("2006-01-02"), 0, 0, now); err != nil {
					log.Printf("daily reconciliation failed: %v", err)
				} else {
					lastDay = day
				}
			}
			if _, err := RunDailyReconciliation(ctx, day, 0, 0, now); err != nil {
				log.Printf("current-day reconciliation failed: %v", err)
			}
			lastRun = now
		}
		if now.Sub(lastPrune) >= time.Hour {
			if err := PruneAdministrationAudit(ctx, now); err != nil {
				log.Printf("audit retention failed: %v", err)
			} else {
				lastPrune = now
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
