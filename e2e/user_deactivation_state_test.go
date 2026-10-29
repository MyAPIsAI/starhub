package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"starhub/model"
)

func TestDeactivationPreviewRouteGuardsOwnerAndDoesNotChangeMoney(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "deactivate-preview-http")
	_, other := exportTestUser(t, env.base, "deactivate-preview-other")
	if err := model.DB.Create(&model.Wallet{UserID: owner, Balance: 9_999_999}).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/user/deactivation-state?expected_user_id=%d", owner)
	for _, test := range []struct {
		cookie string
		status int
	}{{session, 200}, {other, 409}, {env.session, 409}, {"", 401}} {
		req, _ := http.NewRequest(http.MethodGet, env.base+path, nil)
		if test.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: test.cookie})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != test.status || len(res.Cookies()) != 0 || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatalf("invalid readonly preview: %d %s", res.StatusCode, raw)
		}
		if test.status == 200 {
			var got struct {
				Data struct {
					Schema   string `json:"schema"`
					Enabled  bool   `json:"guarded_deactivation"`
					Owner    int64  `json:"user_id"`
					Balance  string `json:"wallet_balance_nano"`
					Ack      bool   `json:"requires_dust_acknowledgement"`
					Revision string `json:"account_revision"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &got) != nil || got.Data.Schema != "deactivation_state_v1" || got.Data.Enabled || got.Data.Owner != owner || got.Data.Balance != "9999999" || !got.Data.Ack || len(got.Data.Revision) != 64 {
				t.Fatal("unexpected preview contract")
			}
		}
		if strings.Contains(string(raw), session) || strings.Contains(string(raw), "password") {
			t.Fatal("preview exposed session/password material")
		}
	}
	var wallet model.Wallet
	model.DB.First(&wallet, "user_id = ?", owner)
	var entries int64
	model.DB.Model(&model.LedgerEntry{}).Where("user_id = ?", owner).Count(&entries)
	user, _ := model.GetUserByID(owner)
	if wallet.Balance != 9_999_999 || entries != 0 || user.Status != 1 {
		t.Fatal("readonly preview changed money or account state")
	}
}
