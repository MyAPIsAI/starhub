package controller

import (
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"starhub/common"
	"starhub/model"
)

func resetSetupState() {
	setupCompleted = false
	setupCompletedOnce = sync.Once{}
}

// newTestDBNoSeed creates a test DB without seeding the root user,
// so setup tests can verify the initial state.
func newTestDBNoSeed(t *testing.T) {
	t.Helper()
	if err := model.InitDB(filepath.Join(t.TempDir(), "setup.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	resetSetupState()
}

func TestGetSetupNeeded(t *testing.T) {
	newTestDBNoSeed(t)

	rr := call(GetSetupHandler, testRequest{target: "/api/setup"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	decodeData(t, rr, &got)
	if got["setup_needed"] != true {
		t.Fatal("setup should be needed when no root user exists")
	}
}

func TestPostSetupCreatesRoot(t *testing.T) {
	newTestDBNoSeed(t)

	rr := call(PostSetupHandler, testRequest{
		method: http.MethodPost,
		target: "/api/setup",
		body:   `{"username":"admin","password":"admin12345"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	users, _ := model.ListUsers()
	foundRoot := false
	for _, u := range users {
		if u.Role == common.RoleRoot {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatal("root user was not created")
	}
}

func TestPostSetupAlreadyCompleted(t *testing.T) {
	newTestDBNoSeed(t)

	call(PostSetupHandler, testRequest{
		method: http.MethodPost,
		target: "/api/setup",
		body:   `{"username":"root1","password":"root12345"}`,
	})

	rr := call(PostSetupHandler, testRequest{
		method: http.MethodPost,
		target: "/api/setup",
		body:   `{"username":"root2","password":"root12345"}`,
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on second setup, got %d", rr.Code)
	}
}

func TestPostSetupValidation(t *testing.T) {
	newTestDBNoSeed(t)

	cases := []struct {
		body string
		want int
	}{
		{`{"username":"a","password":"valid123"}`, http.StatusBadRequest},
		{`{"username":"validuser","password":"short"}`, http.StatusBadRequest},
		{`{"username":"validuser","password":"12345678"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		rr := call(PostSetupHandler, testRequest{
			method: http.MethodPost,
			target: "/api/setup",
			body:   tc.body,
		})
		if rr.Code != tc.want {
			t.Errorf("body %q: expected %d, got %d", tc.body, tc.want, rr.Code)
		}
	}
}
