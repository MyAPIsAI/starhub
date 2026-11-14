package model

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNetworkRoutingConcurrentFirstStateAndSnapshot(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", "")
			if err := db.AutoMigrate(&LocalRoutingState{}, &RoutingSnapshotRecord{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(&LocalRoutingState{}, &RoutingSnapshotRecord{}, &RoutingWriteGuard{})
				ResetRoutingSnapshot()
			})
			if err := initRoutingWriter(db); err != nil {
				t.Fatal(err)
			}
			ResetRoutingSnapshot()
			now := time.Now().UTC().Truncate(time.Second)
			snapshot := testRouteSnapshot(now)
			if err := ApplyRoutingSnapshot(snapshot, now); err != nil {
				t.Fatal(err)
			}
			items, err := snapshot.Validate(now)
			if err != nil {
				t.Fatal(err)
			}
			attempt := &RoutingAttempt{Item: items[0]}
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 12 {
						if err := RecordRoutingOutcome(attempt, false, 503, 0, now); err != nil {
							t.Error(err)
							return
						}
					}
				})
			}
			wg.Go(func() {
				for range 12 {
					if err := ApplyRoutingSnapshot(snapshot, now); err != nil {
						t.Error(err)
						return
					}
				}
			})
			wg.Wait()
			state, err := ReadLocalRoutingState(attempt.Item)
			if err != nil || state.FailureGeneration != 96 || state.ConsecutiveFailures != 96 || state.OpenUntil == 0 {
				t.Fatal("lost circuit transitions", state, err)
			}
			ResetRoutingSnapshot()
			if err := ApplyRoutingSnapshot(snapshot, now); err != nil {
				t.Fatal(err)
			}
			after, err := ReadLocalRoutingState(attempt.Item)
			if err != nil || after != state {
				t.Fatal("snapshot replay changed durable state", after, err)
			}
			var claimed atomic.Int64
			probes := make(chan RoutingProbe, 8)
			probeAt := now.Add(time.Minute)
			for range 8 {
				wg.Go(func() {
					rows, err := ClaimLocalRoutingProbes(probeAt)
					if err != nil {
						t.Error(err)
						return
					}
					claimed.Add(int64(len(rows)))
					for _, row := range rows {
						probes <- row
					}
				})
			}
			wg.Wait()
			if claimed.Load() != 1 {
				t.Fatal("probe lease was claimed more than once", claimed.Load())
			}
			if err := CompleteLocalRoutingProbe(<-probes, true, probeAt); err != nil {
				t.Fatal(err)
			}
			attempt.FailureGeneration = state.FailureGeneration
			if err := RecordRoutingOutcome(attempt, true, 200, 10*time.Millisecond, probeAt); err != nil {
				t.Fatal(err)
			}
			state, err = ReadLocalRoutingState(attempt.Item)
			if err != nil || state.OpenUntil != 0 || state.EWMA != 10 || state.Samples != 1 {
				t.Fatal("recovery or synchronous EWMA failed", state, err)
			}
		})
	}
}
