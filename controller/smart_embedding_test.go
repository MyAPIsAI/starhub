package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestSmartEmbeddingCapabilityAdmission(t *testing.T) {
	testSmartCapabilityAdmission(t, "/v1/embeddings", "embedding", `"input":"review"`, RelayEmbeddings)
}

func TestSmartRerankCapabilityAdmission(t *testing.T) {
	testSmartCapabilityAdmission(t, "/v1/rerank", "rerank", `"query":"review","documents":["review document"]`, RelayRerank)
}

func testSmartCapabilityAdmission(t *testing.T, path, capability, fields string, relayHandler http.HandlerFunc) {
	t.Helper()
	verifiedModel := "verified-" + capability
	for _, tc := range []struct {
		name, requested, limits string
		known                   bool
		want                    int
	}{
		{"canonical unknown", "myapis-auto", "", false, 503},
		{"legacy unknown", "veyra-auto", "", false, 503},
		{"known candidate", "myapis-auto", "", true, 200},
		{"legacy known candidate", "veyra-auto", "", true, 200},
		{"unknown-only whitelist", "myapis-auto", "unverified", true, 403},
		{"explicit unknown model", "unverified", "unverified", false, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			var received atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				var payload struct {
					Model     string   `json:"model"`
					Query     string   `json:"query"`
					Documents []string `json:"documents"`
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Error(err)
				}
				received.Store(payload.Model)
				if r.URL.Path != path || r.Method != "POST" {
					t.Errorf("unexpected relay endpoint: %s %s", r.Method, r.URL.Path)
				}
				if capability == "rerank" && (payload.Query != "review" || len(payload.Documents) != 1 || payload.Documents[0] != "review document") {
					t.Errorf("rerank input was changed: %s", body)
				}
				w.Header().Set("Content-Type", "application/json")
				response := `{"object":"list","data":[],"usage":{"prompt_tokens":12,"total_tokens":12}}`
				if capability == "rerank" {
					response = `{"results":[{"index":0,"relevance_score":0.9}],"usage":{"input_tokens":12,"output_tokens":0}}`
				}
				_, _ = w.Write([]byte(response))
			}))
			defer upstream.Close()
			if err := model.InsertChannel(&model.Channel{Name: "capability-admission", Type: channel.ChannelTypeOpenAI, Models: "unverified," + verifiedModel, Group: "default", Status: common.ChannelStatusEnabled, BaseURL: upstream.URL, Key: "local-review"}); err != nil {
				t.Fatal(err)
			}
			if tc.known {
				if err := model.UpsertModelMeta(&model.ModelMeta{Model: verifiedModel, Tags: capability}); err != nil {
					t.Fatal(err)
				}
			}
			if err := setting.UpdateRatio(setting.OptionKeyModelRatio, fmt.Sprintf(`{"unverified":0.01,%q:1}`, verifiedModel)); err != nil {
				t.Fatal(err)
			}
			u := newUser(t, "embedding-admission", common.RoleUser, 1000000)
			tk := mustToken(t, u.ID, 1000000)
			if tc.limits != "" {
				if err := model.DB.Model(&model.Token{}).Where("id = ?", tk.ID).Update("model_limits", tc.limits).Error; err != nil {
					t.Fatal(err)
				}
			}
			r := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"`+tc.requested+`",`+fields+`}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+tk.Key)
			rec := httptest.NewRecorder()
			middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(relayHandler)).ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body)
			}
			after, err := model.GetUserByID(u.ID)
			if err != nil {
				t.Fatal(err)
			}
			keyAfter, err := model.GetTokenByID(tk.ID)
			if err != nil {
				t.Fatal(err)
			}
			logs, total, err := model.ListLogs(u.ID, 1, 10, "")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != 200 {
				if calls.Load() != 0 || total != 0 || after.Quota != u.Quota || after.UsedQuota != 0 || keyAfter.RemainQuota != tk.RemainQuota || keyAfter.UsedQuota != 0 {
					t.Fatalf("rejected capability request forwarded or charged: calls=%d logs=%+v user=%+v token=%+v", calls.Load(), logs, after, keyAfter)
				}
				return
			}
			wantModel := verifiedModel
			if tc.requested == "unverified" {
				wantModel = "unverified"
			}
			if calls.Load() != 1 || received.Load() != wantModel || total != 1 || len(logs) != 1 || logs[0].Model != wantModel || logs[0].RequestedModel != tc.requested || logs[0].Quota != relay.ComputeQuota(wantModel, "default", 12, 0) || logs[0].PromptTokens != 12 || logs[0].CompletionTokens != 0 || after.Quota != u.Quota-logs[0].Quota || keyAfter.RemainQuota != tk.RemainQuota-logs[0].Quota {
				t.Fatalf("incorrect admitted request: model=%v logs=%+v calls=%d", received.Load(), logs, calls.Load())
			}
		})
	}
}
