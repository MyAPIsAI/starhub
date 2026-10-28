package controller

import (
	"net/http"
	"starhub/common"
	"testing"
)

func TestTokenSmartAccessContract(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "smart-contract", common.RoleUser, 1000000)
	for _, tc := range []struct {
		limits  string
		allowed bool
	}{
		{"", true}, {"gpt-4o", true}, {"myapis-auto", false}, {"veyra-auto", false}, {"myapis-auto,veyra-auto", false}, {"veyra-auto,gpt-4o", true},
	} {
		var created map[string]any
		decodeData(t, call(CreateToken, testRequest{method: http.MethodPost, target: "/api/token", body: mustJSON(t, map[string]any{"name": "review", "model_limits": tc.limits}), userID: user.ID, role: common.RoleUser}), &created)
		if created["smart_access"] != tc.allowed {
			t.Fatalf("create capability mismatch: limits=%q got=%v", tc.limits, created["smart_access"])
		}
		var listed []map[string]any
		decodeData(t, call(ListTokens, testRequest{target: "/api/token", userID: user.ID, role: common.RoleUser}), &listed)
		if len(listed) == 0 || listed[0]["smart_access"] != tc.allowed || listed[0]["model_limits"] != tc.limits {
			t.Fatalf("list changed permission: %+v", listed)
		}
		if listed[0]["key"] == created["key"] {
			t.Fatal("list leaked a plaintext key")
		}
	}
}
