package model

import (
	"testing"
	"time"
)

func TestPublicStatusHourlyWindowsAndDST(t *testing.T) {
	for _, tc := range []struct {
		date, now string
		hours     int
	}{{"2026-11-01", "2026-11-02T00:30:00-05:00", 25}, {"2026-03-08", "2026-03-09T00:30:00-04:00", 23}} {
		t.Run(tc.date, func(t *testing.T) {
			statusFixture(t)
			at, _ := time.Parse(time.RFC3339, tc.now)
			loc, _ := time.LoadLocation("America/New_York")
			start, _ := time.ParseInLocation("2006-01-02", tc.date, loc)
			statusCalls(t, start, "hourly", "highest", 1, 0, "", 100, 100)
			statusCalls(t, start.Add(2*time.Hour), "hourly", "highest", 0, 99, "CONNECT_FAIL", 100, 100)
			statusCalls(t, start, "hourly", "highest", 0, 100, "CLIENT_ERROR", 100, 100)
			statusCalls(t, start, "hourly", "low", 500, 0, "", 100, 100)
			statusCalls(t, at.Add(time.Second), "hourly", "highest", 100, 0, "", 100, 100)
			catalog, err := ReadServiceGroupCatalog(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			groups := StatusPageGroups(catalog)
			data, err := PublicStatusHours(t.Context(), "hourly", "America/New_York", tc.date, groups, at)
			if err != nil || len(data.Hours) != tc.hours || data.Samples != 100 || *data.SuccessRate != 0.01 {
				t.Fatal(data, err)
			}
			if data.Hours[0].Samples != 1 || *data.Hours[0].SuccessRate != 1 || data.Hours[1].SuccessRate != nil || *data.Hours[2].SuccessRate != 0 {
				t.Fatal(data.Hours)
			}
			for i := 1; i < len(data.Hours); i++ {
				if data.Hours[i].Start != data.Hours[i-1].End {
					t.Fatal("overlap or gap")
				}
			}
			window, err := PublicStatusHours(t.Context(), "hourly", "America/New_York", "", groups, at)
			if err != nil || len(window.Hours) != 24 {
				t.Fatal(window, err)
			}
			history, err := PublicStatusHistory(t.Context(), map[string]bool{"hourly": true}, groups, "America/New_York", at)
			if err != nil || window.Samples != history["hourly"].Last24Hours.Samples {
				t.Fatal(window, history, err)
			}
			for _, date := range []string{"bad", "2027-01-01", "2025-01-01"} {
				if _, err := PublicStatusHours(t.Context(), "hourly", "America/New_York", date, groups, at); err != ErrInvalidStatusDate {
					t.Fatal(date, err)
				}
			}
		})
	}
}

func TestPublicStatusFilterDecisionMatchesHistory(t *testing.T) {
	at := statusFixture(t)
	statusCalls(t, at, "outage", "highest", 45, 5, "CONNECT_FAIL", 100, 100)
	statusCalls(t, at, "slow", "highest", 50, 0, "", 2000, 100)
	statusCalls(t, at, "few", "highest", 49, 0, "", 100, 100)
	statusCalls(t, at, "normal", "highest", 100, 0, "", 100, 100)
	catalog, err := ReadServiceGroupCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	groups := StatusPageGroups(catalog)
	names := map[string]bool{"outage": true, "slow": true, "few": true, "normal": true, "empty": true}
	current, err := PublicStatusCurrent(t.Context(), names, groups, at)
	if err != nil {
		t.Fatal(err)
	}
	history, err := PublicStatusHistory(t.Context(), names, groups, "Asia/Dubai", at)
	if err != nil {
		t.Fatal(err)
	}
	for name := range names {
		if current[name].State != history[name].Current.State || current[name].Samples != history[name].Current.Samples {
			t.Fatal(name, current[name], history[name].Current)
		}
	}
}
