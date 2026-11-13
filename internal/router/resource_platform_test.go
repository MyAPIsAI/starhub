package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"starhub/model"
	"starhub/setting"
)

func TestResourceSnapshotWireReceiptAndStartupGate(t *testing.T) {
	if err := model.InitDB(filepath.Join(t.TempDir(), "resource.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB(); setting.ResetAllToDefaults() })
	t.Cleanup(setting.ResetResourceProviderModeToDefault)
	token := strings.Repeat("s", 32)
	t.Setenv("METRIC_AUTHORIZATION", token)
	_ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, "commercial")
	handler := New()
	call := func(method, path, bearer string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call("POST", "/v1/chat/completions", "", nil); w.Code != 503 {
		t.Fatalf("startup gate: %d", w.Code)
	}
	s := model.RouteSnapshot{SnapshotVersion: "v1", ScoreAlgorithmVersion: "five-dimension-v1", RatingAlgorithmVersion: "un-in-v1", PublishedAt: time.Now().UTC(), Items: []json.RawMessage{}}
	s.Checksum = s.PolicyChecksum()
	body := map[string]any{"route_snapshot": s}
	if w := call("POST", "/api/metric/runtime/summary", "wrong", body); w.Code != 401 {
		t.Fatalf("service auth: %d", w.Code)
	}
	w := call("POST", "/api/metric/runtime/summary", token, body)
	var response struct {
		Success bool `json:"success"`
		Receipt struct {
			Applied  bool   `json:"applied"`
			Shared   bool   `json:"shared"`
			Checksum string `json:"checksum"`
		} `json:"protection_receipt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !response.Success || !response.Receipt.Applied || !response.Receipt.Shared || response.Receipt.Checksum != s.Checksum {
		t.Fatalf("publisher receipt mismatch: %s", w.Body)
	}
	if w := call("POST", "/v1/chat/completions", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("gate did not reopen: %d", w.Code)
	}
	if w := call("GET", "/api/channel/models/details", token, nil); w.Code != 200 {
		t.Fatalf("catalog endpoint: %s", w.Body)
	}
	w = call("GET", "/api/metric/routing/policy-status", token, nil)
	var status struct {
		Data struct {
			Instances []struct {
				Version string `json:"snapshot_version"`
				Shared  bool   `json:"shared"`
			} `json:"instances"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(status.Data.Instances) != 1 || status.Data.Instances[0].Version != "v1" || !status.Data.Instances[0].Shared {
		t.Fatalf("policy status: %s", w.Body)
	}
}
