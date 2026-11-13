package service

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// Admission and reads also refresh lazily, so the worker is not a correctness
// dependency at midnight or after the process was stopped across a boundary.
func RunTokenPeriodRollover(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := model.RefreshDueTokenPeriods(time.Now(), 0); err != nil {
			log.Printf("token quota period rollover failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
