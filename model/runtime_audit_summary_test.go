package model

import (
	"context"
	"testing"
	"time"
)

func TestRuntimeAuditSeparatesRequestsAttemptsAndRoutingTime(t *testing.T) {
	newTestDB(t)
	end := time.Now().Truncate(time.Minute).Unix()
	for _, r := range []struct {
		id            string
		status        int
		reject, smart bool
		attempts      int64
		us            int64
	}{{"ok", 1, false, true, 2, 2000}, {"reject", 2, true, false, 0, 0}, {"fail", 2, false, true, 1, 10000}} {
		DB.Create(&EventAuditRequest{RequestID: r.id, StartedAt: end - 90, CompletedAt: end - 60, ExpectedAttempts: r.attempts, ExpectedDecisions: boolCount(!r.reject)})
		DB.Create(&Log{RequestID: r.id, RequestObserved: true, CreatedAt: end - 60, Status: r.status, AdmissionRejected: r.reject})
		if !r.reject {
			DB.Create(&EventAuditDecision{ID: r.id, RequestID: r.id, CompletedAt: end - 60, DurationUS: r.us, Smart: r.smart})
		}
	}
	s, err := ReadRuntimeAuditSummary(context.Background(), end-1800, end)
	if err != nil {
		t.Fatal(err)
	}
	if s.ClientCount != 3 || s.AdmittedCount != 2 || s.SmartCount != 2 || s.ClientSuccess == nil || *s.ClientSuccess < 33 || *s.ClientSuccess > 34 || *s.AdmittedSuccess != 50 || *s.SmartSuccess != 50 || *s.RoutingP95 != 10 {
		t.Fatalf("wrong populations: %+v", s)
	}
}

func boolCount(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
