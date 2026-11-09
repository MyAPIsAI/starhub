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
	lastRun := time.Time{}
	lastPrune := time.Time{}
	for {
		profile, err := CurrentDeploymentProfile(ctx)
		if err == nil && profile.Revision > 0 {
			_ = config.SetRegionTimezone(profile.Profile.Timezone)
		}
		now := time.Now()
		if now.Sub(lastRun) >= time.Minute {
			if err := RunScheduledReconciliation(ctx, now); err != nil {
				log.Printf("daily reconciliation failed: %v", err)
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
