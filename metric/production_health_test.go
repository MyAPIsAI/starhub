package metric

import (
	"strings"
	"testing"
	"time"
)

func TestProductionMemoryAdmissionReleaseAndExpiryAreObservable(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Now().Truncate(time.Minute)
	record := func(generation string) {
		s := BeginRequest(RequestMeta{Model: "m"})
		a := s.BeginAttempt(7)
		a.SetRouteMetadata(AttemptMetadata{Generation: generation})
		a.MarkSent()
		s.EndAttempt(a, AttemptSuccess, 200)
		s.EndRequest(200)
	}
	StartProduction(now.Add(-time.Minute))
	record(strings.Repeat("x", 128<<10))
	h := ProductionCollectorSnapshot()
	if h.Rejected != 1 || h.Retained != 0 || h.EstimatedBytes != 0 {
		t.Fatal(h)
	}
	production.bytes.Store(ProductionMemoryBudget - 100)
	record("g")
	h = ProductionCollectorSnapshot()
	if h.Rejected != 2 || h.Retained != 0 {
		t.Fatal(h)
	}
	production.bytes.Store(0)
	record("g")
	h = ProductionCollectorSnapshot()
	if h.Retained != 1 || h.EstimatedBytes <= 0 {
		t.Fatal(h)
	}
	stage := NextProductionStage(128)
	AckProductionStage(stage)
	h = ProductionCollectorSnapshot()
	if h.Retained != 0 || h.EstimatedBytes <= 0 {
		t.Fatal("aggregate budget was released early", h)
	}
	AckProduction(ClosedProductionMinutes(now.Add(time.Minute)))
	if h = ProductionCollectorSnapshot(); h.EstimatedBytes != 0 {
		t.Fatal("sealed budget leaked", h)
	}
	record("g")
	ClosedProductionMinutes(now.Add(25 * time.Hour))
	h = ProductionCollectorSnapshot()
	if h.Expired != 1 || h.ExpiredBytes <= 0 || h.Retained != 0 || h.EstimatedBytes != 0 {
		t.Fatal("silent expiry", h)
	}
}
