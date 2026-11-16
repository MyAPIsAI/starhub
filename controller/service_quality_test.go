package controller

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestGetServiceQualityCalendarMonth(t *testing.T) {
	for _, monthName := range []string{"2024-01", "2024-02", "2023-02", "2023-12"} {
		t.Run(monthName, func(t *testing.T) {
			newTestDB(t)
			owner := newUser(t, "month-owner", common.RoleUser, 1000)
			other := newUser(t, "month-other", common.RoleUser, 1000)
			start, err := time.Parse("2006-01", monthName)
			if err != nil {
				t.Fatal(err)
			}
			end := start.AddDate(0, 1, 0)
			entries := []model.Log{
				{UserID: owner.ID, Model: "model-a", Status: 1, Quota: 99, DurationMs: 90000, CreatedAt: start.Unix() - 1},
				{UserID: owner.ID, Model: "model-a", Status: 1, Quota: 3, DurationMs: 100, CreatedAt: start.Unix()},
				{UserID: owner.ID, Model: "model-a", Status: 2, DurationMs: 200, CreatedAt: start.Add(time.Hour).Unix()},
				{UserID: owner.ID, Model: "model-a", Status: 1, Quota: 5, DurationMs: 300, CreatedAt: end.Unix() - 1},
				{UserID: owner.ID, Model: "model-b", Status: 1, Quota: 7, DurationMs: 900, CreatedAt: start.Add(2 * time.Hour).Unix()},
				{UserID: other.ID, Model: "model-a", Status: 1, Quota: 11, DurationMs: 5000, CreatedAt: start.Add(3 * time.Hour).Unix()},
				{UserID: owner.ID, Model: "model-a", Status: 1, Quota: 99, DurationMs: 90000, CreatedAt: end.Unix()},
			}
			if err := model.DB.Create(&entries).Error; err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name, filter, user         string
				role                       int
				total, success, quota, p95 int64
			}{
				{"self", "", strconv.FormatInt(other.ID, 10), common.RoleUser, 4, 3, 15, 900},
				{"self model", "model-a", strconv.FormatInt(other.ID, 10), common.RoleUser, 3, 2, 8, 300},
				{"all accounts", "", "", common.RoleAdmin, 5, 4, 26, 5000},
				{"selected account", "model-a", strconv.FormatInt(other.ID, 10), common.RoleAdmin, 1, 1, 11, 5000},
				{"selected root account", "model-a", strconv.FormatInt(owner.ID, 10), common.RoleRoot, 3, 2, 8, 300},
				{"unknown model", "missing", "", common.RoleUser, 0, 0, 0, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					query := url.Values{"month": {monthName}, "hours": {"1"}, "model": {tc.filter}, "user_id": {tc.user}}
					rr := call(GetServiceQuality, testRequest{method: http.MethodGet, target: "/api/service-quality?" + query.Encode(), userID: owner.ID, role: tc.role})
					var got model.ServiceQualitySummary
					decodeData(t, rr, &got)
					if got.WindowStart != start.Unix() || got.WindowEnd != end.Unix() || got.WindowHours != int(end.Sub(start)/time.Hour) {
						t.Fatalf("wrong calendar window: %+v", got)
					}
					if got.Overall.TotalRequests != tc.total || got.Overall.SuccessCount != tc.success || got.Overall.FailedCount != tc.total-tc.success || got.Overall.TotalQuota != tc.quota || got.Overall.P95DurationMs != tc.p95 {
						t.Fatalf("wrong monthly metrics: %+v, want %+v", got, tc)
					}
					var dayTotal, daySuccess, modelTotal int64
					for _, day := range got.ByDay {
						if day.Date < start.Format("2006-01-02") || day.Date >= end.Format("2006-01-02") || day.P95DurationMs > tc.p95 {
							t.Fatalf("out-of-scope daily metrics: %+v", got.ByDay)
						}
						dayTotal += day.TotalRequests
						daySuccess += day.SuccessCount
					}
					for _, metric := range got.ByModel {
						if tc.filter != "" && metric.Model != tc.filter {
							t.Fatalf("other models included: %+v", got.ByModel)
						}
						modelTotal += metric.TotalRequests
					}
					if dayTotal != tc.total || daySuccess != tc.success || modelTotal != tc.total {
						t.Fatalf("daily/model scope differs from overall: %+v", got)
					}
					if tc.total > 0 && (len(got.ByDay) != 2 && tc.total != 1) {
						t.Fatalf("first/last month days were truncated: %+v", got.ByDay)
					}
				})
			}
		})
	}
}

func TestGetServiceQualityCurrentAndInvalidMonth(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "current-month-owner", common.RoleUser, 1000)
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	entries := []model.Log{
		{UserID: owner.ID, Model: "current", Status: 1, CreatedAt: now.Unix()},
		{UserID: owner.ID, Model: "future", Status: 1, CreatedAt: now.Add(time.Hour).Unix()},
	}
	if err := model.DB.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	rr := call(GetServiceQuality, testRequest{method: http.MethodGet, target: "/api/service-quality?month=" + now.Format("2006-01"), userID: owner.ID, role: common.RoleUser})
	var got model.ServiceQualitySummary
	decodeData(t, rr, &got)
	if got.WindowStart != start.Unix() || got.WindowEnd < now.Unix()+1 || got.WindowEnd > time.Now().UTC().Unix()+1 || got.Overall.TotalRequests != 1 || len(got.ByModel) != 1 || got.ByModel[0].Model != "current" {
		t.Fatalf("current month must stop at query time: %+v", got)
	}
	for _, month := range []string{"", "2024-1", "2024-13", "2024-01-01", "1969-12", "0000-01", start.AddDate(0, 1, 0).Format("2006-01")} {
		rr := call(GetServiceQuality, testRequest{method: http.MethodGet, target: "/api/service-quality?month=" + url.QueryEscape(month), userID: owner.ID, role: common.RoleUser})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid month %q: status=%d body=%s", month, rr.Code, rr.Body.String())
		}
	}
}

func TestGetServiceQualityDefault(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	// Insert some logs.
	model.InsertLog(&model.Log{UserID: u.ID, Model: "gpt-4o", Quota: 10, Status: 1, DurationMs: 500})
	model.InsertLog(&model.Log{UserID: u.ID, Model: "gpt-4o", Quota: 20, Status: 1, DurationMs: 700})
	model.InsertLog(&model.Log{UserID: u.ID, Model: "gpt-4o", Quota: 0, Status: 2, DurationMs: 300})
	model.InsertLog(&model.Log{UserID: u.ID, Model: "claude-3", Quota: 5, Status: 1, DurationMs: 900})

	rr := call(GetServiceQuality, testRequest{
		method: http.MethodGet,
		target: "/api/service-quality",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	overall := got["overall"].(map[string]any)
	if overall["total_requests"].(float64) != 4 {
		t.Fatalf("total_requests = %v, want 4", overall["total_requests"])
	}
	if overall["success_count"].(float64) != 3 {
		t.Fatalf("success_count = %v, want 3", overall["success_count"])
	}
	if overall["failed_count"].(float64) != 1 {
		t.Fatalf("failed_count = %v, want 1", overall["failed_count"])
	}
	byModel := got["by_model"].([]any)
	if len(byModel) != 2 {
		t.Fatalf("by_model length = %d, want 2", len(byModel))
	}
}

func TestGetServiceQualityUserIsolation(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	bob := newUser(t, "bob", common.RoleUser, 1000)

	model.InsertLog(&model.Log{UserID: alice.ID, Model: "gpt-4o", Quota: 10, Status: 1})
	model.InsertLog(&model.Log{UserID: bob.ID, Model: "gpt-4o", Quota: 10, Status: 1})

	rr := call(GetServiceQuality, testRequest{
		method: http.MethodGet,
		target: "/api/service-quality",
		userID: alice.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	overall := got["overall"].(map[string]any)
	if overall["total_requests"].(float64) != 1 {
		t.Fatalf("alice total_requests = %v, want 1 (user isolation)", overall["total_requests"])
	}
}

func TestGetServiceQualityAdminSeesAll(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	model.InsertLog(&model.Log{UserID: alice.ID, Model: "gpt-4o", Quota: 10, Status: 1})

	rr := call(GetServiceQuality, testRequest{
		method: http.MethodGet,
		target: "/api/service-quality",
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	overall := got["overall"].(map[string]any)
	if overall["total_requests"].(float64) != 1 {
		t.Fatalf("admin total_requests = %v, want 1", overall["total_requests"])
	}
}

func TestGetServiceQualityEmpty(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(GetServiceQuality, testRequest{
		method: http.MethodGet,
		target: "/api/service-quality",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	overall := got["overall"].(map[string]any)
	if overall["total_requests"].(float64) != 0 {
		t.Fatalf("total_requests = %v, want 0 for empty", overall["total_requests"])
	}
	byModel := got["by_model"].([]any)
	if len(byModel) != 0 {
		t.Fatalf("by_model length = %d, want 0", len(byModel))
	}
}

func TestGetServiceQualityAccountScope(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "quality-owner", common.RoleUser, 1000)
	other := newUser(t, "quality-other", common.RoleUser, 1000)
	for _, entry := range []model.Log{
		{UserID: owner.ID, Model: "owned-model", Status: 1, DurationMs: 100},
		{UserID: other.ID, Model: "other-model", Status: 2, DurationMs: 900},
		{UserID: other.ID, Model: "other-model", Status: 1, DurationMs: 300},
	} {
		if err := model.InsertLog(&entry); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleTech, 11, 101, common.RoleAdmin, common.RoleRoot} {
		for _, filter := range []string{"", strconv.FormatInt(other.ID, 10), "0", "-1", "invalid", "9223372036854775808"} {
			t.Run(strconv.Itoa(role)+"/user_id="+filter, func(t *testing.T) {
				admin := common.IsAdminRole(role)
				rr := call(GetServiceQuality, testRequest{
					method: http.MethodGet,
					target: "/api/service-quality?hours=168&user_id=" + url.QueryEscape(filter),
					userID: owner.ID,
					role:   role,
				})
				if admin && filter != "" && filter != strconv.FormatInt(other.ID, 10) {
					if rr.Code != http.StatusBadRequest {
						t.Fatalf("invalid administrator filter: status=%d body=%s", rr.Code, rr.Body.String())
					}
					return
				}
				var got model.ServiceQualitySummary
				decodeData(t, rr, &got)
				wantTotal, wantModels, wantP95 := int64(1), 1, int64(100)
				if admin {
					wantTotal, wantModels, wantP95 = 3, 2, 900
					if filter != "" {
						wantTotal, wantModels = 2, 1
					}
				}
				if got.Overall.TotalRequests != wantTotal || len(got.ByModel) != wantModels || got.Overall.P95DurationMs != wantP95 {
					t.Fatalf("wrong account totals: %+v", got)
				}
				if len(got.ByDay) != 1 || got.ByDay[0].TotalRequests != wantTotal || got.ByDay[0].P95DurationMs != wantP95 {
					t.Fatalf("daily account scope differs from totals: %+v", got.ByDay)
				}
				if !admin && got.ByModel[0].Model != "owned-model" {
					t.Fatalf("non-administrator saw another account: %+v", got.ByModel)
				}
				if admin && filter != "" && got.ByModel[0].Model != "other-model" {
					t.Fatalf("administrator filter was ignored: %+v", got.ByModel)
				}
			})
		}
	}
	rr := call(GetServiceQuality, testRequest{
		method: http.MethodGet, target: "/api/service-quality?user_id=999999", userID: owner.ID, role: common.RoleAdmin,
	})
	var empty model.ServiceQualitySummary
	decodeData(t, rr, &empty)
	if empty.Overall.TotalRequests != 0 || len(empty.ByModel) != 0 || len(empty.ByDay) != 0 {
		t.Fatalf("unknown user must not fall back to all users: %+v", empty)
	}
}

func TestGetServiceQualityModelFilter(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "quality-alice", common.RoleUser, 1000)
	bob := newUser(t, "quality-bob", common.RoleUser, 1000)
	now := time.Now().UTC()
	const literalModel = "model' OR 1=1 --"
	entries := []model.Log{
		{UserID: alice.ID, Model: "gpt-4o", Status: 1, Quota: 23, DurationMs: 300, CreatedAt: now.AddDate(0, 0, -20).Unix()},
		{UserID: alice.ID, Model: "gpt-4o", Status: 2, DurationMs: 500, CreatedAt: now.Unix()},
		{UserID: alice.ID, Model: "gpt-4o", Status: 1, Quota: 50, DurationMs: 10, CreatedAt: now.AddDate(0, 0, -31).Unix()},
		{UserID: alice.ID, Model: "gpt-4.1", Status: 1, Quota: 11, DurationMs: 200, CreatedAt: now.Unix()},
		{UserID: alice.ID, Model: literalModel, Status: 1, Quota: 7, DurationMs: 100, CreatedAt: now.Unix()},
		{UserID: bob.ID, Model: "gpt-4o", Status: 1, Quota: 99, DurationMs: 900, CreatedAt: now.Unix()},
	}
	for i := range entries {
		if err := model.InsertLog(&entries[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name     string
		model    string
		hours    int
		role     int
		total    int64
		success  int64
		failed   int64
		quota    int64
		duration int64
		days     int
	}{
		{"user thirty days", "gpt-4o", 720, common.RoleUser, 2, 1, 1, 23, 400, 2},
		{"user seven days", "gpt-4o", 168, common.RoleUser, 1, 0, 1, 0, 500, 1},
		{"admin thirty days", "gpt-4o", 720, common.RoleAdmin, 3, 2, 1, 122, 566, 2},
		{"literal model", literalModel, 720, common.RoleUser, 1, 1, 0, 7, 100, 1},
		{"unknown model", "missing-model", 720, common.RoleUser, 0, 0, 0, 0, 0, 0},
		{"unfiltered compatibility", "", 720, common.RoleUser, 4, 3, 1, 41, 275, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := url.Values{"hours": {strconv.Itoa(tc.hours)}}
			if tc.model != "" {
				query.Set("model", tc.model)
			}
			// A user-controlled scope parameter must not widen normal-user access.
			if !common.IsAdminRole(tc.role) {
				query.Set("user_id", "0")
			}
			rr := call(GetServiceQuality, testRequest{
				method: http.MethodGet, target: "/api/service-quality?" + query.Encode(),
				userID: alice.ID, role: tc.role,
			})
			var got model.ServiceQualitySummary
			decodeData(t, rr, &got)
			overall := got.Overall
			if got.WindowHours != tc.hours || overall.TotalRequests != tc.total || overall.SuccessCount != tc.success || overall.FailedCount != tc.failed || overall.TotalQuota != tc.quota || overall.AvgDurationMs != tc.duration {
				t.Fatalf("filtered summary = %+v, want %+v", got, tc)
			}
			if tc.total > 0 {
				if overall.SuccessRate != float64(tc.success)/float64(tc.total) || overall.ErrorRate != 1-overall.SuccessRate {
					t.Fatalf("filtered success/error rates are wrong: %+v", overall)
				}
			}
			if tc.model != "" {
				wantModels := 1
				if tc.total == 0 {
					wantModels = 0
				}
				if len(got.ByModel) != wantModels || (wantModels == 1 && got.ByModel[0].Model != tc.model) {
					t.Fatalf("other models leaked into filtered metrics: %+v", got.ByModel)
				}
				if wantModels == 1 && (got.ByModel[0].AvgDurationMs != tc.duration || got.ByModel[0].TotalRequests != tc.total || got.ByModel[0].TotalQuota != tc.quota) {
					t.Fatalf("per-model metrics disagree with filtered totals: %+v", got.ByModel)
				}
			}
			var requests, successes, failures int64
			for _, day := range got.ByDay {
				if day.SuccessRate != float64(day.SuccessCount)/float64(day.TotalRequests) {
					t.Fatalf("daily success rate is wrong: %+v", day)
				}
				requests += day.TotalRequests
				successes += day.SuccessCount
				failures += day.FailedCount
			}
			if len(got.ByDay) != tc.days || requests != tc.total || successes != tc.success || failures != tc.failed {
				t.Fatalf("daily trend disagrees with filtered totals: %+v", got.ByDay)
			}
		})
	}
}

func TestGetServiceQualityP95(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "p95-alice", common.RoleUser, 1000)
	bob := newUser(t, "p95-bob", common.RoleUser, 1000)
	now := time.Now().UTC()
	var logs []model.Log
	for i := int64(1); i <= 20; i++ {
		status := 1
		if i == 20 {
			status = 2
		}
		logs = append(logs,
			model.Log{UserID: alice.ID, Model: "model-a", DurationMs: i * 100, Status: status, CreatedAt: now.Unix()},
			model.Log{UserID: alice.ID, Model: "model-a", DurationMs: 0, CreatedAt: now.Unix()},
			model.Log{UserID: alice.ID, Model: "model-b", DurationMs: 10000, CreatedAt: now.Unix()},
		)
	}
	logs = append(logs,
		model.Log{UserID: alice.ID, Model: "model-a", DurationMs: 2100, CreatedAt: now.AddDate(0, 0, -20).Unix()},
		model.Log{UserID: alice.ID, Model: "model-a", DurationMs: 1000000, CreatedAt: now.AddDate(0, 0, -31).Unix()},
		model.Log{UserID: bob.ID, Model: "model-a", DurationMs: 50000, CreatedAt: now.Unix()},
		model.Log{UserID: alice.ID, Model: "unmeasured", DurationMs: 0, CreatedAt: now.Unix()},
		model.Log{UserID: alice.ID, Model: "unmeasured", DurationMs: 0, CreatedAt: now.Unix()},
		model.Log{UserID: alice.ID, Model: "unmeasured", DurationMs: -5, CreatedAt: now.Unix()},
		model.Log{UserID: alice.ID, Model: "single", DurationMs: 4321, CreatedAt: now.Unix()},
		model.Log{UserID: alice.ID, Model: "", DurationMs: 777, CreatedAt: now.Unix()},
	)
	if err := model.DB.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		model   string
		hours   int
		role    int
		total   int64
		overall int64
		byModel map[string]int64
	}{
		{"all models", "", 168, common.RoleUser, 65, 10000, map[string]int64{"model-a": 1900, "model-b": 10000, "unmeasured": 0, "single": 4321, "": 777}},
		{"twenty one samples", "model-a", 720, common.RoleUser, 41, 2000, map[string]int64{"model-a": 2000}},
		{"twenty samples", "model-a", 168, common.RoleUser, 40, 1900, map[string]int64{"model-a": 1900}},
		{"admin scope", "model-a", 720, common.RoleAdmin, 42, 2100, map[string]int64{"model-a": 2100}},
		{"no measurements", "unmeasured", 168, common.RoleUser, 3, 0, map[string]int64{"unmeasured": 0}},
		{"one measurement", "single", 168, common.RoleUser, 1, 4321, map[string]int64{"single": 4321}},
		{"empty result", "missing", 168, common.RoleUser, 0, 0, map[string]int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := url.Values{"hours": {strconv.Itoa(tc.hours)}, "model": {tc.model}}
			rr := call(GetServiceQuality, testRequest{method: http.MethodGet, target: "/api/service-quality?" + query.Encode(), userID: alice.ID, role: tc.role})
			var got model.ServiceQualitySummary
			decodeData(t, rr, &got)
			if got.Overall.P95DurationMs != tc.overall || got.Overall.TotalRequests != tc.total || len(got.ByModel) != len(tc.byModel) {
				t.Fatalf("P95 or request scope is wrong: %+v, want %+v", got, tc)
			}
			for _, metric := range got.ByModel {
				want, ok := tc.byModel[metric.Model]
				if !ok || metric.P95DurationMs != want {
					t.Fatalf("per-model P95 is wrong: %+v, want %d", metric, want)
				}
			}
		})
	}
}

func TestGetServiceQualityDailyP95(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "daily-p95-alice", common.RoleUser, 1000)
	bob := newUser(t, "daily-p95-bob", common.RoleUser, 1000)
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	previous := midnight.Add(-time.Second)
	unmeasured := midnight.AddDate(0, 0, -2)
	older := midnight.AddDate(0, 0, -20)
	var logs []model.Log
	for i := int64(1); i <= 20; i++ {
		status := 1
		if i == 20 {
			status = 2
		}
		logs = append(logs,
			model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: i * 100, Status: status, CreatedAt: previous.Unix()},
			model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: 0, Status: 1, CreatedAt: previous.Unix()},
			model.Log{UserID: alice.ID, Model: "daily-b", DurationMs: 10000, Status: 1, CreatedAt: previous.Unix()},
		)
	}
	logs = append(logs,
		model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: -5, Status: 1, CreatedAt: previous.Unix()},
		model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: 9000, Status: 1, CreatedAt: midnight.Unix()},
		model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: 4321, Status: 1, CreatedAt: older.Unix()},
		model.Log{UserID: alice.ID, Model: "daily-a", DurationMs: 1000000, Status: 1, CreatedAt: midnight.AddDate(0, 0, -31).Unix()},
		model.Log{UserID: bob.ID, Model: "daily-a", DurationMs: 40000, Status: 1, CreatedAt: previous.Unix()},
		model.Log{UserID: alice.ID, Model: "unmeasured", DurationMs: 0, Status: 1, CreatedAt: unmeasured.Unix()},
		model.Log{UserID: alice.ID, Model: "unmeasured", DurationMs: -5, Status: 2, CreatedAt: unmeasured.Unix()},
	)
	if err := model.DB.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	type expectedDay struct {
		date  time.Time
		total int64
		p95   int64
	}
	for _, tc := range []struct {
		name  string
		model string
		hours int
		role  int
		days  []expectedDay
	}{
		{"all models", "", 168, common.RoleUser, []expectedDay{{unmeasured, 2, 0}, {previous, 61, 10000}, {midnight, 1, 9000}}},
		{"one model", "daily-a", 168, common.RoleUser, []expectedDay{{previous, 41, 1900}, {midnight, 1, 9000}}},
		{"thirty days", "daily-a", 720, common.RoleUser, []expectedDay{{older, 1, 4321}, {previous, 41, 1900}, {midnight, 1, 9000}}},
		{"admin scope", "daily-a", 168, common.RoleAdmin, []expectedDay{{previous, 42, 2000}, {midnight, 1, 9000}}},
		{"no measurements", "unmeasured", 168, common.RoleUser, []expectedDay{{unmeasured, 2, 0}}},
		{"empty result", "missing", 168, common.RoleUser, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := url.Values{"hours": {strconv.Itoa(tc.hours)}, "model": {tc.model}}
			if !common.IsAdminRole(tc.role) {
				query.Set("user_id", "0")
			}
			rr := call(GetServiceQuality, testRequest{method: http.MethodGet, target: "/api/service-quality?" + query.Encode(), userID: alice.ID, role: tc.role})
			var got map[string]any
			decodeData(t, rr, &got)
			days := got["by_day"].([]any)
			if len(days) != len(tc.days) {
				t.Fatalf("daily rows = %+v, want %+v", days, tc.days)
			}
			var total int64
			for i, want := range tc.days {
				day := days[i].(map[string]any)
				// Check the wire field even when the measured value is zero.
				if day["date"] != want.date.Format("2006-01-02") || day["p95_duration_ms"] != float64(want.p95) || day["total_requests"] != float64(want.total) {
					t.Fatalf("daily metric = %+v, want %+v", day, want)
				}
				total += want.total
			}
			if got["overall"].(map[string]any)["total_requests"] != float64(total) {
				t.Fatalf("daily counts disagree with overall: %+v", got)
			}
		})
	}
}
