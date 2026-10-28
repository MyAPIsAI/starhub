package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestUnknownNumericRolesCannotReadOrManageOtherAccounts(t *testing.T) {
	env := setupRelayApp(t)
	actor := &model.User{Username: "role-owner", Role: common.RoleUser, Status: common.StatusEnabled, Group: "default", Quota: 5000, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(actor); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(actor.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	ownedToken := &model.Token{UserID: actor.ID, Name: "Owned key", Status: common.StatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	if err := model.InsertToken(ownedToken); err != nil {
		t.Fatal(err)
	}
	orders := map[int64]*model.RechargeOrder{}
	exports := map[int64]*model.ExportRecord{}
	for _, id := range []int64{env.rootID, actor.ID} {
		marker := "other-private-marker"
		if id == actor.ID {
			marker = "owned-private-marker"
		}
		if _, err := model.GetOrCreateWallet(id); err != nil {
			t.Fatal(err)
		}
		if err := model.DB.Create(&model.LedgerEntry{UserID: id, Type: "adjustment", Description: marker}).Error; err != nil {
			t.Fatal(err)
		}
		order := &model.RechargeOrder{UserID: id, Amount: 10_000_000_000}
		if err := model.CreateRechargeOrder(order); err != nil {
			t.Fatal(err)
		}
		orders[id] = order
		export := &model.ExportRecord{UserID: id, Type: "requests", Content: []byte("model\n" + marker + "\n")}
		if err := model.InsertExport(export); err != nil {
			t.Fatal(err)
		}
		exports[id] = export
		if err := model.InsertLog(&model.Log{UserID: id, Model: marker, DurationMs: 100, PromptTokens: 10, Quota: 1, Status: 1}); err != nil {
			t.Fatal(err)
		}
	}
	rootBefore, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	rootKey, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 11, 99, 101, 1000, common.RoleAdmin, common.RoleRoot, 99} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			// Each case verifies authorization rather than accumulated IP throttling.
			middleware.ResetRateLimits()
			if err := model.UpdateUserFields(actor.ID, map[string]any{"role": role}); err != nil {
				t.Fatal(err)
			}
			admin := common.IsAdminRole(role)
			storedActor, err := model.GetUserByID(actor.ID)
			if err != nil || storedActor.IsAdmin() != admin {
				t.Fatal("model authority disagrees with assigned role")
			}
			for _, path := range []string{"/api/token", "/api/ledger", "/api/recharge", "/api/export"} {
				response := doRequest(env.base, "GET", path, session, nil)
				var body struct {
					Data []struct {
						UserID int64 `json:"user_id"`
					} `json:"data"`
				}
				want := 1
				if admin {
					want = 2
				}
				if err := json.Unmarshal([]byte(response.Body), &body); err != nil || response.Status != 200 || len(body.Data) != want {
					t.Fatalf("role %d %s list scope: %d %s %v", role, path, response.Status, response.Body, err)
				}
				for _, row := range body.Data {
					if !admin && row.UserID != actor.ID {
						t.Fatalf("role %d %s disclosed another owner", role, path)
					}
				}
			}
			for _, path := range []string{"/api/log", "/api/service-quality?hours=24"} {
				response := doRequest(env.base, "GET", path, session, nil)
				if response.Status != 200 || !strings.Contains(response.Body, "owned-private-marker") || (!admin && strings.Contains(response.Body, "other-private-marker")) {
					t.Fatalf("role %d %s leaked or lost private observations: %d %s", role, path, response.Status, response.Body)
				}
			}
			usage := doRequest(env.base, "GET", fmt.Sprintf("/api/user/usage?user_id=%d", env.rootID), session, nil)
			wanted := "owned-private-marker"
			if admin {
				wanted = "other-private-marker"
			}
			if usage.Status != 200 || !strings.Contains(usage.Body, wanted) || (!admin && strings.Contains(usage.Body, "other-private-marker")) {
				t.Fatalf("role %d usage query bypassed ownership: %d %s", role, usage.Status, usage.Body)
			}
			crossWant := 403
			if admin {
				crossWant = 200
			}
			for _, path := range []string{fmt.Sprintf("/api/recharge/%d", orders[env.rootID].ID), fmt.Sprintf("/api/export/%d/download", exports[env.rootID].ID)} {
				if response := doRequest(env.base, "GET", path, session, nil); response.Status != crossWant {
					t.Fatalf("role %d cross-account %s: %d", role, path, response.Status)
				}
			}
			if !admin {
				for _, request := range []struct {
					method, path string
					body         any
				}{
					{"PUT", "/api/token", map[string]any{"id": env.tokenID, "name": "Must not change"}},
					{"DELETE", fmt.Sprintf("/api/token/%d", env.tokenID), nil},
					{"POST", fmt.Sprintf("/api/token/%d/regenerate", env.tokenID), nil},
					{"POST", fmt.Sprintf("/api/recharge/%d/begin", orders[env.rootID].ID), nil},
					{"POST", fmt.Sprintf("/api/recharge/%d/resolve", orders[env.rootID].ID), map[string]any{"outcome": "success"}},
				} {
					if response := doRequest(env.base, request.method, request.path, session, request.body); response.Status != 403 {
						t.Fatalf("role %d could mutate another owner's resource: %s %s status=%d", role, request.method, request.path, response.Status)
					}
				}
			}
			if response := doRequest(env.base, "PUT", "/api/token", session, map[string]any{"id": ownedToken.ID, "name": "Owned key"}); response.Status != 200 {
				t.Fatal("ordinary owner lost own-token editing")
			}
			if response := doRequest(env.base, "GET", fmt.Sprintf("/api/export/%d/download", exports[actor.ID].ID), session, nil); response.Status != 200 || !strings.Contains(response.Body, "owned-private-marker") {
				t.Fatal("ordinary owner lost own export access")
			}
		})
	}
	rootAfter, err := model.GetUserByID(env.rootID)
	if err != nil || rootAfter.Quota != rootBefore.Quota || rootAfter.UsedQuota != rootBefore.UsedQuota {
		t.Fatal("authorization checks changed another account's quota")
	}
	afterKey, err := model.GetTokenByID(env.tokenID)
	if err != nil || afterKey.Key != rootKey.Key || afterKey.Name != rootKey.Name {
		t.Fatal("forbidden operations changed another account's token")
	}
	afterOrder, err := model.GetRechargeOrder(orders[env.rootID].ID)
	if err != nil || afterOrder.Status != model.RechargePending {
		t.Fatal("forbidden operations changed another account's order")
	}
	for _, id := range []int64{env.rootID, actor.ID} {
		wallet, err := model.GetOrCreateWallet(id)
		if err != nil || wallet.Balance != 0 {
			t.Fatal("authorization checks credited a wallet")
		}
	}
}
