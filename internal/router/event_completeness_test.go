package router

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestEventCompletenessRequiresServiceIdentity(t *testing.T) {
	if err := model.InitDB(filepath.Join(t.TempDir(), "audit.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB(); setting.ResetAllToDefaults() })
	key := strings.Repeat("x", 32)
	t.Setenv("METRIC_AUTHORIZATION", key)
	handler := New()
	for _, tc := range []struct {
		bearer string
		status int
	}{{"", 401}, {"customer-key", 401}, {key, 200}} {
		r := httptest.NewRequest("GET", "/api/metric/events/completeness", nil)
		if tc.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
	}
}
