package metric

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProductionSealsExactGenerationsAndJournalWithoutCustomerData(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	StartProduction(now.Add(-time.Minute))
	for _, generation := range []string{"old", "new"} {
		s := BeginRequest(RequestMeta{UserID: 77, TokenID: 99, Model: "m", Stream: true})
		a := s.BeginAttempt(1)
		a.SetRouteMetadata(AttemptMetadata{Generation: generation, ResourceKey: "1:m", Capability: "chat", Protocol: "OpenAI", Group: "default", Affinity: "hit"})
		a.MarkSent()
		a.RecordUsage(12, 3)
		a.RecordFirstToken()
		a.ttft = 50 * time.Millisecond
		s.EndAttempt(a, AttemptSuccess, 200)
		s.EndAttempt(a, AttemptSuccess, 200)
	}
	batch := ClosedProductionMinutes(now.Add(time.Minute))
	if len(batch) != 2 || batch[0].Complete || !batch[1].Complete || len(batch[1].Entries) != 2 || len(batch[1].Attempts) != 2 {
		t.Fatalf("incorrect seal: %+v", batch)
	}
	for _, e := range batch[1].Entries {
		if e.Counts.N != 1 || e.Counts.S != 1 || e.Health.TTFTSum != 50 {
			t.Fatalf("bad counts: %+v", e)
		}
	}
	bytes, _ := json.Marshal(batch)
	if strings.Contains(string(bytes), "user_id") || strings.Contains(string(bytes), "token_id") || strings.Contains(string(bytes), "prompt") {
		t.Fatal("customer data in telemetry")
	}
	id := batch[1].BatchID
	if next := ClosedProductionMinutes(now.Add(time.Minute)); next[1].BatchID != id {
		t.Fatal("retry changed batch identity")
	}
	AckProduction(batch)
	if next := ClosedProductionMinutes(now.Add(time.Minute)); len(next) != 0 {
		t.Fatal("acknowledged batch replayed")
	}
}

func TestProductionRotationIsConcurrentAndRetrySnapshotsAreDetached(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	StartProduction(now.Add(-time.Minute))
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s := BeginRequest(RequestMeta{Model: "parallel"})
				a := s.BeginAttempt(int64(id + 1))
				a.SetRouteMetadata(AttemptMetadata{Generation: "g", RequestedCapabilities: []string{"chat"}})
				a.MarkSent()
				s.EndAttempt(a, AttemptSuccess, 200)
			}
		}(worker)
	}
	ClosedProductionMinutes(now.Add(time.Minute))
	wg.Wait()
	batch := ClosedProductionMinutes(now.Add(2 * time.Minute))
	ids := map[string]bool{}
	for _, b := range batch {
		for _, a := range b.Attempts {
			if ids[a.ID] {
				t.Fatal("duplicate attempt after rotation")
			}
			ids[a.ID] = true
		}
	}
	if len(ids) != 1600 {
		t.Fatalf("lost attempts: %d", len(ids))
	}
	before, _ := json.Marshal(batch)
	for i := range batch {
		if len(batch[i].Attempts) > 0 {
			batch[i].Attempts[0].RequestedCapabilities[0] = "modified"
		}
		if len(batch[i].Entries) > 0 {
			batch[i].Entries[0].Health.N = 999999
		}
	}
	after, _ := json.Marshal(ClosedProductionMinutes(now.Add(2 * time.Minute)))
	if string(before) != string(after) {
		t.Fatal("caller mutated canonical sealed retry data")
	}
}

func TestProductionUpdatesExistingResourcesAtCardinalityLimit(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	StartProduction(now.Add(-time.Minute))
	record := func(id int64) {
		s := BeginRequest(RequestMeta{Model: "model"})
		a := s.BeginAttempt(id)
		a.SetRouteMetadata(AttemptMetadata{Generation: fmt.Sprint(id)})
		a.MarkSent()
		s.EndAttempt(a, AttemptSuccess, 200)
	}
	for i := 1; i <= 4096; i++ {
		record(int64(i))
	}
	record(4096)

	record(4097)
	batch := ClosedProductionMinutes(now.Add(time.Minute))
	last := batch[len(batch)-1]
	if last.Complete || last.Error != "collector_capacity_exceeded" || len(last.Entries) != 4096 || len(last.Attempts) != 4097 {
		t.Fatal("new key limit was not enforced")
	}
}

func TestProductionUnknownAndCapacityNeverBecomeAcquiredZeros(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	StartProduction(now.Add(-time.Minute))
	s := BeginRequest(RequestMeta{Model: "m"})
	a := s.BeginAttempt(1)
	s.EndAttempt(a, AttemptSuccess, 200)
	batch := ClosedProductionMinutes(now.Add(time.Minute))
	if batch[1].Complete || batch[1].Error != "missing_endpoint_identity" {
		t.Fatal("missing identity produced full coverage")
	}
	Reset()
	StartProduction(now.Add(-time.Minute))
	production.retained.Store(100000)
	s = BeginRequest(RequestMeta{Model: "m"})
	a = s.BeginAttempt(1)
	a.SetRouteMetadata(AttemptMetadata{Generation: "g"})
	s.EndAttempt(a, AttemptSuccess, 200)
	batch = ClosedProductionMinutes(now.Add(time.Minute))
	if batch[1].Complete || batch[1].Error != "collector_capacity_exceeded" {
		t.Fatal("overflow produced full coverage")
	}
}

func TestProductionUncertainAndCustomerErrorsRemainMetadataOnly(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	StartProduction(now.Add(-time.Minute))
	for _, outcome := range []AttemptClass{AttemptUncertain, AttemptClientError} {
		s := BeginRequest(RequestMeta{Model: "m"})
		a := s.BeginAttempt(1)
		a.SetRouteMetadata(AttemptMetadata{Generation: "g"})
		a.MarkSent()
		s.EndAttempt(a, outcome, 400)
	}
	batch := ClosedProductionMinutes(now.Add(time.Minute))
	if len(batch[1].Attempts) != 2 || len(batch[1].Entries) != 0 || !batch[1].Complete {
		t.Fatal("unconfirmed outcomes became provider failures")
	}
}
