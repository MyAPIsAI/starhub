package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/model"
)

func TestUIAC08KeyTotalsMatchHistoryAndExports(t *testing.T) {
	env, owner, session, first, second := usageAcceptanceAccount(t)
	other, otherSession := exportTestUser(t, env.base, "report-other")
	now := time.Now().Unix()
	for i := 0; i < 1201; i++ {
		key := first.ID
		if i%2 == 1 {
			key = second.ID
		}
		status := 1
		if i%3 == 0 {
			status = 2
		}
		row := model.Log{UserID: owner, TokenID: key, TokenName: "captured-key", Model: "report-model", Vendor: "Vendor A", ServiceGroup: "default", CallType: "chat", CreatedAt: now, Status: status, Quota: int64(i + 1), PromptTokens: 10, CompletionTokens: 5}
		if err := model.InsertLog(&row); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.InsertLog(&model.Log{UserID: other, Model: "private-other-model", Quota: 999999}); err != nil {
		t.Fatal(err)
	}
	for _, dim := range []string{"key", "model", "vendor", "call_type", "group"} {
		t.Run(dim, func(t *testing.T) {
			r := doRequest(env.base, "GET", "/api/user/reports?range=7d&group_by="+dim, session, nil)
			var got struct {
				Data model.UsageReport `json:"data"`
			}
			if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 {
				t.Fatalf("report: %d %s", r.Status, r.Body)
			}
			total := new(big.Int)
			var count, success int64
			for _, row := range got.Data.Rows {
				amount, ok := new(big.Int).SetString(row.AmountNano, 10)
				if !ok {
					t.Fatal("invalid exact money")
				}
				total.Add(total, amount)
				count += row.Requests
				success += row.Successful
			}
			if total.String() != got.Data.Totals.AmountNano || count != 1201 || success != got.Data.Totals.Successful || success >= count || strings.Contains(r.Body, "private-other-model") {
				t.Fatalf("dimension totals differ: %s", r.Body)
			}
			if dim == "key" && len(got.Data.Rows) != 2 {
				t.Fatal("Key grouping unavailable")
			}
		})
	}
	report := publicationAPIData[model.UsageReport](t, doRequest(env.base, "GET", "/api/user/reports?range=7d", session, nil))
	var actual big.Int
	var count int64
	for page := 1; ; page++ {
		var data struct {
			Data struct {
				Items []struct {
					Amount string `json:"amount_nano"`
				} `json:"items"`
				Total int64 `json:"total"`
			} `json:"data"`
		}
		r := doRequest(env.base, "GET", fmt.Sprintf("/api/log?scope=owned&p=%d&page_size=100", page), session, nil)
		if json.Unmarshal([]byte(r.Body), &data) != nil || r.Status != 200 {
			t.Fatal("history unavailable")
		}
		for _, row := range data.Data.Items {
			v, _ := new(big.Int).SetString(row.Amount, 10)
			actual.Add(&actual, v)
			count++
		}
		if count >= data.Data.Total {
			break
		}
	}
	if count != 1201 || actual.String() != report.Totals.AmountNano {
		t.Fatal("history and report differ")
	}
	exported := publicationAPIData[struct {
		URL string `json:"download_url"`
		ID  int64  `json:"id"`
	}](t, doRequest(env.base, "POST", "/api/export", session, map[string]any{"type": "usage", "range_days": 7, "group_by": "key", "language": "en"}))
	deadline := time.Now().Add(5 * time.Second)
	var download apiResult
	for time.Now().Before(deadline) {
		download = doRequest(env.base, "GET", exported.URL, session, nil)
		if download.Status == 200 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(download.Body, "\ufeff"))).ReadAll()
	if err != nil || download.Status != 200 || len(rows) != 3 {
		t.Fatalf("report export: %d %s", download.Status, download.Body)
	}
	var exportedTotal big.Int
	for _, row := range rows[1:] {
		v, _ := new(big.Int).SetString(row[len(row)-1], 10)
		exportedTotal.Add(&exportedTotal, v)
	}
	if exportedTotal.String() != report.Totals.AmountNano {
		t.Fatal("export and report totals differ")
	}
	if r := doRequest(env.base, "GET", "/api/user/reports?key_id="+fmt.Sprint(first.ID)+"&user_id="+fmt.Sprint(owner), otherSession, nil); r.Status != 200 || strings.Contains(r.Body, "report-model") {
		t.Fatal("foreign Key filter crossed accounts")
	}
}

func TestUI34ExactLargeMoneyAndMixedSuccess(t *testing.T) {
	env, owner, session, first, _ := usageAcceptanceAccount(t)
	max := int64(1<<63 - 1)
	for i := 0; i < 2; i++ {
		if err := model.InsertLog(&model.Log{UserID: owner, TokenID: first.ID, Model: "large", Quota: max, Status: 2}); err != nil {
			t.Fatal(err)
		}
	}
	report := publicationAPIData[model.UsageReport](t, doRequest(env.base, "GET", "/api/user/reports?range=7d", session, nil))
	want := new(big.Int).Mul(big.NewInt(max), big.NewInt(4000)).String()
	if report.Totals.AmountNano != want || report.Totals.Successful != 0 {
		t.Fatalf("large aggregate: %+v", report.Totals)
	}
}

func TestUIAC09And10GatewayTraceSwitchAndFrozenBill(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.001")
	upstream := newMockUpstream(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"synthetic failure"}}`))
	}))
	t.Cleanup(bad.Close)
	for _, ch := range []*model.Channel{{Name: "first", Type: 1, Status: 1, Key: upstreamKey, BaseURL: bad.URL, Models: "prd-model", Group: "default", Priority: 100}, {Name: "second", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "prd-model", Group: "default", Priority: 1}} {
		if err := model.InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.DB.Create(&model.ModelMeta{Model: "prd-model", Vendor: "example", VendorDisplay: "Captured Vendor"}).Error; err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(keyPRDBody(false))
	req, _ := http.NewRequest("POST", env.base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(response.Body)
	response.Body.Close()
	trace := response.Header.Get("X-Request-ID")
	if response.StatusCode != 200 || trace == "" {
		t.Fatalf("gateway response %d", response.StatusCode)
	}
	if err := model.InsertLog(&model.Log{UserID: env.rootID, TokenID: env.tokenID, Model: trace, TraceID: strings.Repeat("f", 64)}); err != nil {
		t.Fatal(err)
	}
	r := doRequest(env.base, "GET", "/api/log?scope=owned&trace_id="+trace, env.session, nil)
	var got struct {
		Data struct {
			Total int64                        `json:"total"`
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || got.Data.Total != 1 {
		t.Fatalf("exact header lookup: %d %s", r.Status, r.Body)
	}
	row := got.Data.Items[0]
	var switched bool
	_ = json.Unmarshal(row["auto_switched"], &switched)
	var vendor, group, bill string
	_ = json.Unmarshal(row["vendor"], &vendor)
	_ = json.Unmarshal(row["service_group"], &group)
	_ = json.Unmarshal(row["billing_detail"], &bill)
	if !switched || vendor != "Captured Vendor" || group != "default" || row["channel_id"] != nil || row["used_channels"] != nil || bill == "" {
		t.Fatalf("public metadata missing or internal metadata exposed: %s", r.Body)
	}
	var detail model.BillingDetail
	if json.Unmarshal([]byte(bill), &detail) != nil || detail.Mode != "request" || detail.Parts[0].UnitPriceUSD != "0.001" {
		t.Fatalf("captured price: %s", bill)
	}
	before := doRequest(env.base, "GET", "/api/log/"+trace, env.session, nil)
	keyPRDConfigure(t, env, "0.099")
	after := doRequest(env.base, "GET", "/api/log/"+trace, env.session, nil)
	if before.Status != 200 || before.Body != after.Body {
		t.Fatal("historical billing changed after repricing")
	}
	_, other := exportTestUser(t, env.base, "trace-other")
	if r := doRequest(env.base, "GET", "/api/log/"+trace, other, nil); r.Status != 404 {
		t.Fatal("foreign trace exposed")
	}
}

func usageAcceptanceAccount(t *testing.T) (p1Env, int64, string, *model.Token, *model.Token) {
	t.Helper()
	env := setupRelayApp(t)
	id, session := exportTestUser(t, env.base, "ui-report-audit")
	keys := []*model.Token{
		{UserID: id, Name: "audit-key-one", Status: common.StatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserID: id, Name: "audit-key-two", Status: common.StatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
	}
	for _, key := range keys {
		if err := model.InsertToken(key); err != nil {
			t.Fatal(err)
		}
	}
	return env, id, session, keys[0], keys[1]
}

func TestUI34OneYearReport(t *testing.T) {
	env, owner, session, first, _ := usageAcceptanceAccount(t)
	if err := model.InsertLog(&model.Log{UserID: owner, TokenID: first.ID, Model: "historical-model", Quota: 41, CreatedAt: time.Now().AddDate(0, 0, -200).Unix()}); err != nil {
		t.Fatal(err)
	}
	r := doRequest(env.base, "GET", "/api/user/usage?days=365", session, nil)
	var got struct {
		Data struct {
			Days  int                `json:"days"`
			Items []model.DailyUsage `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 {
		t.Fatalf("report unavailable: %d %s", r.Status, r.Body)
	}
	if got.Data.Days != 365 || len(got.Data.Items) != 1 {
		t.Fatalf("365-day report was reduced to %d days and returned %d rows; the 200-day-old record is missing", got.Data.Days, len(got.Data.Items))
	}
}

func TestUI34RegionalCalendar(t *testing.T) {
	previous := config.RegionTimezone()
	if err := config.ApplyStartup(config.Startup{Bootstrap: config.Bootstrap{RegionTimezone: "Asia/Dubai"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ApplyStartup(config.Startup{Bootstrap: config.Bootstrap{RegionTimezone: previous}}) })
	env, owner, session, first, _ := usageAcceptanceAccount(t)
	now := time.Now().UTC()
	at := time.Date(now.Year(), now.Month(), now.Day()-1, 20, 5, 0, 0, time.UTC)
	if err := model.InsertLog(&model.Log{UserID: owner, TokenID: first.ID, Model: "regional-model", Quota: 7, CreatedAt: at.Unix()}); err != nil {
		t.Fatal(err)
	}
	r := doRequest(env.base, "GET", "/api/user/usage?days=7", session, nil)
	var got struct {
		Data struct {
			Items []model.DailyUsage `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 || len(got.Data.Items) != 1 {
		t.Fatalf("report unavailable: %d %s", r.Status, r.Body)
	}
	loc, _ := time.LoadLocation("Asia/Dubai")
	want := at.In(loc).Format("2006-01-02")
	if got.Data.Items[0].Date != want {
		t.Fatalf("regional day should be %s, received UTC day %s", want, got.Data.Items[0].Date)
	}
}

func TestUI35ExactTraceAndDimensionFilters(t *testing.T) {
	env, owner, session, first, second := usageAcceptanceAccount(t)
	trace := strings.Repeat("a", 64)
	for _, row := range []model.Log{
		{UserID: owner, TokenID: first.ID, Model: "shared-model", TraceID: trace, Quota: 11},
		{UserID: owner, TokenID: second.ID, Model: "shared-model", TraceID: strings.Repeat("b", 64), Quota: 23},
		{UserID: owner, TokenID: first.ID, Model: trace, TraceID: strings.Repeat("c", 64), Quota: 7},
	} {
		if err := model.InsertLog(&row); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, query string
		want        int64
	}{
		{"exact response trace", "query=" + trace, 1},
		{"Key filter", "key_id=" + fmt.Sprint(second.ID), 1},
		{"token filter", "token_id=" + fmt.Sprint(second.ID), 1},
		{"model filter", "model=shared-model", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := doRequest(env.base, "GET", "/api/log?scope=owned&"+tc.query, session, nil)
			var got struct {
				Data struct {
					Total int64       `json:"total"`
					Items []model.Log `json:"items"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(r.Body), &got) != nil || r.Status != 200 {
				t.Fatalf("history unavailable: %d %s", r.Status, r.Body)
			}
			if got.Data.Total != tc.want {
				t.Fatalf("%s: received %d rows, require %d", tc.name, got.Data.Total, tc.want)
			}
		})
	}
}
