package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

type arcTenantFixture struct {
	env                p1Env
	aID, bID           int64
	aSession, bSession string
	aKey, bKey         *model.Token
}

func arcTenants(t *testing.T) arcTenantFixture {
	t.Helper()
	f := arcTenantFixture{env: setupRelayApp(t)}
	f.aID, f.aSession = exportTestUser(t, f.env.base, "arc-audit-alice")
	f.bID, f.bSession = exportTestUser(t, f.env.base, "arc-audit-bob")
	f.aKey = &model.Token{UserID: f.aID, Name: "arc-a-key", Status: common.StatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	f.bKey = &model.Token{UserID: f.bID, Name: "arc-b-key", Status: common.StatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	for _, key := range []*model.Token{f.aKey, f.bKey} {
		if err := model.InsertToken(key); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []model.Log{
		{UserID: f.aID, TokenID: f.aKey.ID, Model: "arc-a-private-model", TraceID: "arc_trace_a_private", Quota: 300, Status: 1},
		{UserID: f.bID, TokenID: f.bKey.ID, Model: "arc-b-own-model", TraceID: "arc_trace_b_own", Quota: 7, Status: 1},
	} {
		if err := model.InsertLog(&row); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestARCAC12ForeignTraceAndReport(t *testing.T) {
	f := arcTenants(t)
	t.Run("foreign trace search is empty", func(t *testing.T) {
		path := fmt.Sprintf("/api/log?scope=owned&query=arc_trace_a_private&user_id=%d&key_id=%d", f.aID, f.aKey.ID)
		r := doRequest(f.env.base, "GET", path, f.bSession, nil)
		var got struct {
			Data struct {
				Total int64       `json:"total"`
				Items []model.Log `json:"items"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || got.Data.Total != 0 || len(got.Data.Items) != 0 {
			t.Fatalf("foreign trace lookup: %d %s", r.Status, r.Body)
		}
	})
	for _, keyParam := range []string{"key_id", "token_id"} {
		t.Run("foreign report "+keyParam, func(t *testing.T) {
			path := fmt.Sprintf("/api/user/usage?days=7&user_id=%d&%s=%d", f.aID, keyParam, f.aKey.ID)
			r := doRequest(f.env.base, "GET", path, f.bSession, nil)
			if r.Status == 404 {
				return
			}
			var got struct {
				Data struct {
					Items []model.DailyUsage `json:"items"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || len(got.Data.Items) != 1 || got.Data.Items[0].Quota != 7 || got.Data.Items[0].Requests != 1 || strings.Contains(r.Body, "arc-a-private-model") || !strings.Contains(r.Body, "arc-b-own-model") {
				t.Fatalf("foreign report lookup: %d %s", r.Status, r.Body)
			}
		})
	}
}

func TestARCAC13ExportDownloadContract(t *testing.T) {
	f := arcTenants(t)
	r := doRequest(f.env.base, "POST", "/api/export", f.aSession, map[string]any{"type": "requests", "range_days": 7})
	var got struct {
		Data struct {
			ID  int64  `json:"id"`
			URL string `json:"download_url"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 {
		t.Fatalf("create export: %d %s", r.Status, r.Body)
	}
	t.Run("public URL is opaque", func(t *testing.T) {
		if !regexp.MustCompile(`^/api/export/files/[0-9a-f]{64}/download$`).MatchString(got.Data.URL) {
			t.Fatal("export URL uses a predictable identifier")
		}
	})
	t.Run("owner download works", func(t *testing.T) {
		r := doRequest(f.env.base, "GET", got.Data.URL, f.aSession, nil)
		if r.Status != 200 || !strings.Contains(r.Body, "arc-a-private-model") {
			t.Fatalf("owner download: %d", r.Status)
		}
	})
	t.Run("foreign download returns 404", func(t *testing.T) {
		r := doRequest(f.env.base, "GET", got.Data.URL, f.bSession, nil)
		if r.Status != 404 {
			t.Fatalf("foreign download status: %d", r.Status)
		}
	})
	t.Run("anonymous gets 404 or login redirect", func(t *testing.T) {
		r := doRequest(f.env.base, "GET", got.Data.URL, "", nil)
		if r.Status != 404 && (r.Status < 300 || r.Status >= 400) {
			t.Fatalf("PRD expects 404 or login redirect, got %d", r.Status)
		}
	})
	t.Run("sequential download URL is unavailable", func(t *testing.T) {
		for _, session := range []string{f.aSession, f.bSession, f.env.session, ""} {
			r := doRequest(f.env.base, "GET", fmt.Sprintf("/api/export/%d/download", got.Data.ID), session, nil)
			if r.Status != 404 {
				t.Fatalf("retired sequential-ID URL returned %d", r.Status)
			}
		}
	})
	t.Run("files and dictionaries share private not-found responses", func(t *testing.T) {
		var denied string
		for _, suffix := range []string{"download", "dictionary"} {
			path := strings.TrimSuffix(got.Data.URL, "download") + suffix
			if owner := doRequest(f.env.base, "GET", path, f.aSession, nil); owner.Status != 200 || owner.Body == "" {
				t.Fatalf("owner %s unavailable: %d", suffix, owner.Status)
			}
			for _, session := range []string{f.bSession, f.env.session, "", "invalid-session"} {
				r := doRequest(f.env.base, "GET", path, session, nil)
				if r.Status != 404 || (denied != "" && r.Body != denied) {
					t.Fatalf("private %s denial differs: %d %s", suffix, r.Status, r.Body)
				}
				denied = r.Body
			}
		}
		missing := "/api/export/files/" + strings.Repeat("0", 64) + "/download"
		if r := doRequest(f.env.base, "GET", missing, f.aSession, nil); r.Status != 404 || r.Body != denied {
			t.Fatal("missing file differs from a private file")
		}
		if r := doRequest(f.env.base, "GET", "/api/wallet", "", nil); r.Status != 401 {
			t.Fatal("private file handling changed normal authentication")
		}
	})
}

func arcPaidOrder(t *testing.T, f arcTenantFixture, session string) (int64, string) {
	t.Helper()
	r := doRequest(f.env.base, "POST", "/api/recharge", session, map[string]any{"amount_usd": "10.00"})
	var got struct {
		Data struct {
			ID     int64  `json:"id"`
			Number string `json:"order_number"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || got.Data.ID <= 0 {
		t.Fatalf("create order: %d %s", r.Status, r.Body)
	}
	if _, err := model.BeginRechargePayment(got.Data.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ResolveRechargeOrder(got.Data.ID, "success"); err != nil {
		t.Fatal(err)
	}
	return got.Data.ID, got.Data.Number
}

func TestARCAC15ForeignOrderAndLedger(t *testing.T) {
	f := arcTenants(t)
	aID, aNumber := arcPaidOrder(t, f, f.aSession)
	bID, bNumber := arcPaidOrder(t, f, f.bSession)
	_, unrelatedNumber := arcPaidOrder(t, f, f.bSession)
	before, err := model.GetOrCreateWallet(f.aID)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("foreign numeric order is indistinguishable from missing", func(t *testing.T) {
		foreign := doRequest(f.env.base, "GET", fmt.Sprintf("/api/recharge/%d", aID), f.bSession, nil)
		missing := doRequest(f.env.base, "GET", "/api/recharge/999999999", f.bSession, nil)
		if foreign.Status != 404 || missing.Status != 404 || foreign.Body != missing.Body {
			t.Fatalf("foreign=%d, missing=%d; PRD requires 404", foreign.Status, missing.Status)
		}
	})
	t.Run("foreign order number returns 404", func(t *testing.T) {
		r := doRequest(f.env.base, "GET", "/api/recharge/"+url.PathEscape(aNumber), f.bSession, nil)
		if r.Status != 404 {
			t.Fatalf("foreign business order number: %d %s", r.Status, r.Body)
		}
	})
	for _, param := range []string{"order_id", "order_number"} {
		t.Run("foreign ledger "+param+" returns 404", func(t *testing.T) {
			r := doRequest(f.env.base, "GET", "/api/ledger?"+param+"="+url.QueryEscape(aNumber), f.bSession, nil)
			if r.Status != 404 {
				t.Fatalf("foreign ledger lookup: %d; contains A order=%v; contains B order=%v", r.Status, strings.Contains(r.Body, aNumber), strings.Contains(r.Body, bNumber))
			}
		})
		t.Run("own ledger "+param+" is filtered", func(t *testing.T) {
			for _, ref := range []string{fmt.Sprint(bID), bNumber} {
				r := doRequest(f.env.base, "GET", "/api/ledger?"+param+"="+url.QueryEscape(ref), f.bSession, nil)
				var got struct {
					Data []model.LedgerEntry `json:"data"`
				}
				if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || len(got.Data) != 1 || got.Data[0].OrderID != bNumber || got.Data[0].UserID != f.bID || strings.Contains(r.Body, unrelatedNumber) {
					t.Fatalf("own order filter failed: %d %s", r.Status, r.Body)
				}
			}
		})
	}
	t.Run("owner can read by ID or order number", func(t *testing.T) {
		for _, ref := range []string{fmt.Sprint(bID), bNumber} {
			r := doRequest(f.env.base, "GET", "/api/recharge/"+ref, f.bSession, nil)
			var got struct {
				Data struct {
					ID     int64  `json:"id"`
					UserID int64  `json:"user_id"`
					Number string `json:"order_number"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || got.Data.ID != bID || got.Data.UserID != f.bID || got.Data.Number != bNumber {
				t.Fatalf("owner order lookup failed: %d %s", r.Status, r.Body)
			}
		}
	})
	t.Run("management reads remain explicit about personal scope", func(t *testing.T) {
		for _, path := range []string{"/api/recharge/" + aNumber, "/api/ledger?order_number=" + aNumber} {
			if r := doRequest(f.env.base, "GET", path, f.env.session, nil); r.Status != 200 || !strings.Contains(r.Body, aNumber) {
				t.Fatalf("management read unavailable: %d", r.Status)
			}
			separator := "?"
			if strings.Contains(path, "?") {
				separator = "&"
			}
			if r := doRequest(f.env.base, "GET", path+separator+"scope=owned", f.env.session, nil); r.Status != 404 {
				t.Fatal("management role bypassed personal scope")
			}
		}
	})
	t.Run("invalid and ambiguous selectors never return the whole ledger", func(t *testing.T) {
		for _, query := range []string{"order_id=", "order_id=" + bNumber + "&order_id=" + aNumber, "order_id=" + bNumber + "&order_number=" + bNumber} {
			if r := doRequest(f.env.base, "GET", "/api/ledger?"+query, f.bSession, nil); r.Status != 400 {
				t.Fatalf("invalid selector: %d", r.Status)
			}
		}
		for _, ref := range []string{"999999999", "ord_missing", "0", "01", "not-an-order"} {
			for _, path := range []string{"/api/recharge/" + ref, "/api/ledger?order_id=" + ref} {
				if r := doRequest(f.env.base, "GET", path, f.bSession, nil); r.Status != 404 {
					t.Fatalf("missing order: %d %s", r.Status, r.Body)
				}
			}
		}
	})
	after, err := model.GetOrCreateWallet(f.aID)
	order, orderErr := model.GetRechargeOrder(aID)
	var count int64
	model.DB.Model(&model.LedgerEntry{}).Where("user_id = ?", f.aID).Count(&count)
	if err != nil || orderErr != nil || after.Balance != before.Balance || order.Status != model.RechargePaid || count != 1 {
		t.Fatal("isolation checks changed A's money or order")
	}
}
