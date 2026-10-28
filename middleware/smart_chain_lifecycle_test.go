package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

func TestExplicitSmartContinuationRequiresUnexpiredPin(t *testing.T) {
	smartFixture(t)
	body := `{"model":"myapis-auto","messages":[{"role":"tool","content":"result"}],"max_tokens":8}`
	for _, state := range []string{"missing", "expired", "purged"} {
		if state == "expired" {
			if _, err := model.PinRoutingChain(1, "default", "lifecycle", "capable"); err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.RoutingChain{}).Where("token_id = ?", 1).UpdateColumn("expires_at", time.Now().Unix()).Error; err != nil {
				t.Fatal(err)
			}
		}
		if state == "purged" {
			if err := model.CleanupExpiredRoutingChains(time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
		}
		if code, _ := routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "lifecycle", 1, ""); code != 400 {
			t.Fatalf("%s explicit continuation status=%d", state, code)
		}
	}
}

func TestSmartSelectionDoesNotRenewBinding(t *testing.T) {
	smartFixture(t)
	if _, err := model.PinRoutingChain(1, "default", "no-admission", "cheap"); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Unix() + 60
	if err := model.DB.Model(&model.RoutingChain{}).Where("token_id = ?", 1).UpdateColumn("expires_at", expires).Error; err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"myapis-auto","max_tokens":8}`))
	r.Header.Set("X-MyAPIs-Chain-Id", "no-admission")
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenID, int64(1)))
	Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })).ServeHTTP(httptest.NewRecorder(), r)
	var pin model.RoutingChain
	if err := model.DB.First(&pin).Error; err != nil {
		t.Fatal(err)
	}
	if pin.ExpiresAt != expires {
		t.Fatalf("selection renewed rejected request: %d want=%d", pin.ExpiresAt, expires)
	}
}
