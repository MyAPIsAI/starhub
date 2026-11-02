package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/relay/dto"
)

func TestSmartCandidatePolicyOnlyNarrowsAuthorization(t *testing.T) {
	smartFixture(t)
	for _, test := range []struct {
		name, requested, limits, candidates, want string
		enabled                                   bool
		status                                    int
	}{
		{"narrow candidate", "myapis-auto", "cheap,capable", "capable", "capable", true, 204},
		{"legacy alias", "veyra-auto", "cheap,capable", "capable", "capable", true, 204},
		{"candidate outside allowlist", "myapis-auto", "cheap", "capable", "", true, 403},
		{"smart disabled", "myapis-auto", "cheap,capable", "capable", "", false, 403},
		{"direct remains allowed", "cheap", "cheap,capable", "capable", "cheap", false, 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+test.requested+`","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenModelLimits, test.limits, common.CtxTokenSmartCandidates, test.candidates, common.CtxTokenSmartAccess, test.enabled))
			selected := ""
			response := httptest.NewRecorder()
			Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				selected, _ = r.Context().Value(common.CtxRequestModel).(string)
				w.WriteHeader(204)
			})).ServeHTTP(response, r)
			if response.Code != test.status || selected != test.want {
				t.Fatalf("status=%d selected=%q want %d/%q", response.Code, selected, test.status, test.want)
			}
		})
	}
}
