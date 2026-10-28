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
			query.Set("user_id", "0")
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
