package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEventCompletenessIndependentReceipts(t *testing.T) {
	newTestDB(t)
	now := time.Now().Truncate(time.Minute)
	at := now.Add(-time.Minute)
	for _, id := range []string{"success", "rejected", "missing"} {
		if err := BeginEventAudit(id, at); err != nil {
			t.Fatal(err)
		}
	}
	DB.Create(&Log{RequestID: "success", RequestObserved: true, CreatedAt: at.Unix(), Status: 1})
	DB.Create(&Log{RequestID: "rejected", RequestObserved: true, CreatedAt: at.Unix(), Status: 2, AdmissionRejected: true})
	CompleteEventAudit("success", 1, 2, at)
	CompleteEventAudit("rejected", 0, 0, at)
	CompleteEventAudit("missing", 1, 1, at)
	DB.Create(&EventAuditDecision{ID: "d", RequestID: "success", CompletedAt: at.Unix()})
	DB.Create(&ProductionAttemptRecord{ID: "a", Sent: true, RequestID: "success", Minute: at.Unix(), Payload: "{}"})
	d, err := ReadEventCompleteness(context.Background(), at.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if d.RequestEventsTotal != 3 || d.DecisionEventsTotal != 2 || d.AttemptEventsTotal != 3 || d.RequestCompleteness == nil || *d.RequestCompleteness < 66 || *d.RequestCompleteness > 67 || *d.DecisionCompleteness != 50 || *d.AttemptCompleteness < 33 || *d.AttemptCompleteness > 34 {
		t.Fatalf("wrong denominators: %+v", d)
	}
	DB.Create(&Log{RequestID: "legacy", CreatedAt: at.Unix()})
	d, err = ReadEventCompleteness(context.Background(), at.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if d.UnknownHistoricalRequests != 1 || d.RequestCompleteness != nil {
		t.Fatal("historical unknown became complete")
	}
	raw, _ := json.Marshal(ProductionAttemptRecord{ID: "a", RequestID: "private"})
	if strings.Contains(string(raw), "private") {
		t.Fatal("internal correlation leaked")
	}
}
