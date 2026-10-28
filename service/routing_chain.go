package service

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// RunRoutingChainCleanup reclaims expired state even for inactive tokens.
func RunRoutingChainCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := model.CleanupExpiredRoutingChains(time.Now().Unix()); err != nil {
			log.Printf("cleanup routing chains failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
