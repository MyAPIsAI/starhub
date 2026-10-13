package controller

import (
	"net/http"
	"testing"

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
