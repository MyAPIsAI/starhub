package model

import (
	"sync"
	"testing"
	"time"
)

func TestLocalRoutingConcurrentFailuresSnapshotRefreshAndRestart(t *testing.T) {
	_, snapshot := scoredFixture(t, 1)
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	const perWorker = 12
	errors := make(chan error, workers+1)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Go(func() {
			for j := 0; j < perWorker; j++ {
				if err := RecordRoutingOutcome(ch.RuntimeRoute, false, 503, 0, time.Now()); err != nil {
					errors <- err
					return
				}
			}
		})
	}
	group.Go(func() {
		for i := 0; i < 12; i++ {
			if err := ApplyRoutingSnapshot(snapshot, time.Now()); err != nil {
				errors <- err
				return
			}
		}
	})
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	state, err := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if err != nil || state.FailureGeneration != workers*perWorker || state.OpenUntil == 0 {
		t.Fatalf("lost circuit transitions: %+v %v", state, err)
	}
	var database struct{ File string }
	if err := DB.Raw("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&database).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(database.File); err != nil {
		t.Fatal(err)
	}
	after, err := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if err != nil || after != state {
		t.Fatalf("restart changed circuit state: %+v %+v %v", state, after, err)
	}
}

func TestLocalRoutingSkipsUnchangedSavesButPersistsLatencySynchronously(t *testing.T) {
	_, _ = scoredFixture(t, 1)
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, time.Millisecond, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("CREATE TRIGGER reject_routing_save BEFORE UPDATE ON local_routing_states WHEN NEW.resource_key != '' BEGIN SELECT RAISE(ABORT,'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 0, time.Now()); err != nil {
		t.Fatal("unchanged state was written", err)
	}
	if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, time.Millisecond, time.Now()); err == nil {
		t.Fatal("changed EWMA bypassed durable confirmation")
	}
}
