package controller

import (
	"testing"
	"time"

	"starhub/config"
	"starhub/model"
)

func TestOperatingDashboardRoleAndDateContract(t *testing.T) {
	newTestDB(t)
	previous := config.RegionTimezone()
	if err := config.SetRegionTimezone("Asia/Dubai"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.SetRegionTimezone(previous) })
	for _, role := range []int{1, 20, 99} {
		expectStatus(t, call(GetOperatingDashboard, testRequest{target: "/api/admin/dashboard", role: role, userID: 1}), 403)
	}
	for _, role := range []int{5, 10, 100} {
		var result model.OperatingDashboard
		response := call(GetOperatingDashboard, testRequest{target: "/api/admin/dashboard?range=7d", role: role, userID: 1})
		decodeData(t, response, &result)
		if response.Header().Get("Cache-Control") != "no-store" || len(result.Series) != 7 || result.Timezone != "Asia/Dubai" {
			t.Fatalf("invalid report: %+v", result)
		}
	}
	for _, query := range []string{"range=bad", "range=custom&start_date=2026-02-30&end_date=2026-03-01", "range=custom&start_date=2024-01-01&end_date=2026-01-01", "range=custom&start_date=2026-04-01&end_date=2026-03-01", "range=custom&start_date=2999-01-01&end_date=2999-01-01"} {
		expectStatus(t, call(GetOperatingDashboard, testRequest{target: "/api/admin/dashboard?" + query, role: 100, userID: 1}), 400)
	}
	loc, _ := time.LoadLocation("Asia/Dubai")
	day := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	var result model.OperatingDashboard
	decodeData(t, call(GetOperatingDashboard, testRequest{target: "/api/admin/dashboard?range=custom&start_date=" + day + "&end_date=" + day, role: 100, userID: 1}), &result)
	if len(result.Series) != 24 || result.StartDate != day || result.EndDate != day {
		t.Fatal("custom dates not inclusive regional days")
	}
}
