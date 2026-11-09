package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/dto"
)

func TestPublicPagesPublishedScopeAndProjection(t *testing.T) {
	catalogFixture(t)
	at := time.Now().Truncate(time.Second)
	if err := model.InsertChannel(&model.Channel{Name: "Secret", Key: "private-channel-secret", Type: 1, Status: 1, Group: "private", Models: "private-only"}); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.ServiceGroupConfig{}).Where("group_id = ?", "standard").Update("description", "Published group description").Error; err != nil {
		t.Fatal(err)
	}
	// An outage must remain visible on the status page even after catalog hiding.
	if err := model.DB.Model(&model.Ability{}).Where("model = ?", "public-chat").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err := model.DB.Create(&model.Log{Model: "public-chat", ServiceGroup: "standard", CreatedAt: at.Unix(), Status: 2, ErrorCode: "CONNECT_FAIL", UserID: 423, TokenName: "private-key", TraceID: "private-request"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/public/home", "/api/status/models?page_size=1&q=public-chat"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		if strings.Contains(path, "/home") {
			GetPublicHome(w, r)
		} else {
			publicServiceStatusAt(at)(w, r)
		}
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{"private-only", "private-channel-secret", "private-customer", "private-key", "private-request", "unlisted", "myapis-auto", `"account_ids"`, `"user_id"`, `"token_id"`, `"channel_id"`} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("public response leaked %s", secret)
			}
		}
		if strings.Contains(path, "/home") {
			if !strings.Contains(w.Body.String(), "Published group description") || strings.Contains(w.Body.String(), "Private") {
				t.Fatal(w.Body.String())
			}
		} else {
			var data struct {
				Items []struct {
					ID string
					model.PublicModelStatus
				}
				Total          int
				RefreshSeconds int `json:"refresh_seconds"`
			}
			decodeData(t, w, &data)
			if data.Total != 1 || len(data.Items) != 1 || data.Items[0].Current.State != "outage" || len(data.Items[0].Daily) != 90 || data.RefreshSeconds != 300 || *data.Items[0].Last24Hours.SuccessRate != 0 {
				t.Fatal(w.Body.String())
			}
		}
	}
	w := httptest.NewRecorder()
	publicServiceStatusAt(at)(w, httptest.NewRequest("GET", "/api/status/models?vendor=audio-vendor&q=chat", nil))
	if !strings.Contains(w.Body.String(), `"items":[]`) || !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal(w.Body.String())
	}
}

func TestPublicStatusActualUpstreamErrorDenominator(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			newTestDB(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":{"message":"Synthetic upstream failure","type":"api_error"}}`))
			}))
			defer upstream.Close()
			if err := model.InsertChannel(&model.Channel{Name: "Status failure fixture", Type: 1, Key: "synthetic-upstream-key", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: 1, Weight: 1}); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "status-fixture-user", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat)))
			r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}],"max_tokens":8}`))
			r.Header.Set("Authorization", "Bearer "+token.Key)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code < 400 {
				t.Fatal(w.Code, w.Body.String())
			}
			var logs []model.Log
			if err := model.DB.Find(&logs).Error; err != nil || len(logs) != 1 {
				t.Fatal(logs, err, w.Body.String())
			}
			data, err := model.PublicStatusHistory(t.Context(), map[string]bool{"gpt-4o": true}, []model.ServiceGroupConfig{{GroupID: "default"}}, "Asia/Dubai", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			var samples int64
			if code == 429 || code == 500 {
				samples = 1
			}
			if data["gpt-4o"].Last24Hours.Samples != samples {
				t.Fatal(code, logs[0].ErrorCode, data["gpt-4o"])
			}
			if samples == 1 && (data["gpt-4o"].Last24Hours.SuccessRate == nil || *data["gpt-4o"].Last24Hours.SuccessRate != 0) {
				t.Fatal(data)
			}
		})
	}
}

func TestStatusGroupConfigurationGuardAndValidation(t *testing.T) {
	catalogFixture(t)
	catalog, err := model.ReadServiceGroupCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var group model.ServiceGroupConfig
	for _, g := range catalog.Groups {
		if g.GroupID == "standard" {
			group = g
		}
	}
	body := map[string]any{"group_id": group.GroupID, "name": group.Name, "enabled": true, "visibility": "common", "multiplier": "0.6", "status_page_mode": "exclude", "expected_group_id": group.ID, "expected_revision": group.Revision, "expected_option_version": catalog.OptionVersion}
	send := func() *httptest.ResponseRecorder {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: string(b)})
	}
	w := send()
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status_page_mode":"exclude"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = send(); w.Code != 409 {
		t.Fatal("stale override was accepted", w.Code, w.Body.String())
	}
	delete(body, "expected_group_id")
	delete(body, "expected_revision")
	delete(body, "expected_option_version")
	body["status_page_mode"] = "invalid"
	if w = send(); w.Code != 400 {
		t.Fatal("invalid mode accepted", w.Code, w.Body.String())
	}
	body["status_page_mode"] = "include"
	if w = send(); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	delete(body, "status_page_mode")
	if w = send(); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	GetServiceGroupCatalog(w, httptest.NewRequest("GET", "/api/service-group/configuration", nil))
	if !strings.Contains(w.Body.String(), `"status_page_mode":"include"`) {
		t.Fatal("legacy update erased override", w.Body.String())
	}
}

func TestPublicStatusFiltersBeforePaginationAndHourlyPrivacy(t *testing.T) {
	catalogFixture(t)
	at := time.Now().Truncate(time.Second)
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("filter-%02d", i)
		if err := model.DB.Create(&model.StandardModel{Model: name, Kind: "chat", Vendor: "Fixture", Listed: true}).Error; err != nil {
			t.Fatal(err)
		}
		var logs []model.Log
		for n := 0; n < 50; n++ {
			status, code := 1, ""
			if i >= 22 {
				status, code = 2, "CONNECT_FAIL"
			}
			logs = append(logs, model.Log{Model: name, ServiceGroup: "standard", CreatedAt: at.Unix(), Status: status, ErrorCode: code})
		}
		if err := model.DB.Create(&logs).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query string
		total int
		id    string
	}{{"state=abnormal&page_size=1&page=2", 3, "filter-23"}, {"state=unknown&type=audio", 1, "public-audio"}, {"state=abnormal&type=audio", 0, ""}, {"q=filter-24&type=text&state=abnormal", 1, "filter-24"}} {
		w := httptest.NewRecorder()
		publicServiceStatusAt(at)(w, httptest.NewRequest("GET", "/api/status/models?"+tc.query, nil))
		var body struct {
			Total int
			Items []struct{ ID string }
		}
		decodeData(t, w, &body)
		if body.Total != tc.total || (tc.id != "" && (len(body.Items) == 0 || body.Items[0].ID != tc.id)) {
			t.Fatal(tc, w.Body.String())
		}
	}
	for _, tc := range []struct {
		id, date string
		code     int
	}{{"filter-22", "", 200}, {"unlisted", "", 404}, {"filter-22", "invalid", 400}, {"private-only", "", 404}} {
		r := httptest.NewRequest("GET", "/api/status/models/"+tc.id+"/history?date="+tc.date, nil)
		r.SetPathValue("id", tc.id)
		w := httptest.NewRecorder()
		GetPublicStatusHistory(w, r)
		if w.Code != tc.code {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.code == 200 && (!strings.Contains(w.Body.String(), `"hours"`) || strings.Contains(w.Body.String(), "user_id")) {
			t.Fatal(w.Body.String())
		}
	}
}
