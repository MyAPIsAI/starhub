package relay

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
)

func parseBillingDetail(t *testing.T, raw string) model.BillingDetail {
	t.Helper()
	var detail model.BillingDetail
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Version != 1 {
		t.Fatalf("unsupported detail: %s", raw)
	}
	return detail
}

func TestFrozenMeteringMatchesSettlementModes(t *testing.T) {
	pd := &PriceData{ModelRatio: 2, CompletionRatio: 3, CacheRatio: .25, AudioRatio: 4, GroupRatio: .5, OtherRatio: 1}
	u := &Usage{PromptTokens: 100, CompletionTokens: 50, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 20, CacheWriteTokens: 10, AudioTokens: 5}}
	detail := parseBillingDetail(t, BillingSnapshot(pd, u))
	for i, want := range []struct{ qty, price string }{{"65", "1"}, {"50", "3"}, {"20", "0.25"}, {"10", "1"}, {"5", "4"}, {"0", "4"}} {
		if part := detail.Parts[i]; part.Quantity != want.qty || part.UnitPriceUSD != want.price {
			t.Fatalf("part %d = %+v, want %+v", i, part, want)
		}
	}
	pd.GroupRatio = 0
	for _, part := range parseBillingDetail(t, BillingSnapshot(pd, u)).Parts {
		if part.UnitPriceUSD != "0" {
			t.Fatalf("free group price = %s", part.UnitPriceUSD)
		}
	}
	pd.GroupRatio, pd.PriceExpr, pd.ModelPriceUSD = 1, "unavailable_variable * .01", .02
	if got := parseBillingDetail(t, BillingSnapshot(pd, u)); got.Mode != "request" || got.Formula != "" || got.Parts[0].UnitPriceUSD != "0.02" {
		t.Fatalf("failed expression fallback = %+v", got)
	}
}

func TestImageAndTaskBillingDetailsUseTheirOwnExpressionVariables(t *testing.T) {
	pd := &PriceData{PriceExpr: "count * size / 1024 * quality * .01 + prompt_tokens * .000001", ModelPriceUSD: .02, GroupRatio: 1, OtherRatio: 1}
	b := &ImageBill{Model: "image-model", Prompt: "PRIVATE_PROMPT_MUST_NOT_BE_STORED", Count: 3, Size: "1024x1024", Quality: "standard", Pricing: pd}
	if b.SettleQuota(2, nil) <= 0 {
		t.Fatal("expected image charge")
	}
	detail := parseBillingDetail(t, b.BillingDetail)
	if detail.Mode != "expression" || detail.Parts[0].Quantity != "2" || detail.Parts[3].Quantity == "0" || strings.Contains(b.BillingDetail, b.Prompt) {
		t.Fatalf("image details = %s", b.BillingDetail)
	}
	pd.PriceExpr = "seconds * n * .1"
	p := channel.TaskPricing{PerCallUSD: .02, Units: 6, ExprVars: map[string]float64{"seconds": 3, "n": 2}}
	if got := parseBillingDetail(t, TaskBillingDetails(pd, p)); got.Mode != "expression" {
		t.Fatalf("task details = %+v", got)
	}
	pd.PriceExpr = "missing * .1"
	if got := parseBillingDetail(t, TaskBillingDetails(pd, p)); got.Mode != "unit" || got.Formula != "" || got.Parts[0].Quantity != "6" {
		t.Fatalf("task fallback = %+v", got)
	}
	pd.PriceExpr = "seconds * n * .1"
	for _, status := range []string{"failed", "cancelled"} {
		got := parseBillingDetail(t, PluginBillingDetails(PluginTaskPricingSnapshot{PriceData: *pd, Usage: taskplugin.Usage{Seconds: 3, N: 2}}, taskplugin.Result{Status: status}))
		if got.Mode != "not_charged" || got.Formula != "" || len(got.Parts) != 0 {
			t.Fatalf("%s details = %+v", status, got)
		}
	}
}

func TestInterruptedSettlementsKeepMeteringAndOutcomeIndependent(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{{2, "stream_interrupted"}, {3, "request_cancelled"}} {
		t.Run(tc.code, func(t *testing.T) {
			_, user, key, _ := rebateFixture(t)
			pd := &PriceData{ModelPriceUSD: .0002, GroupRatio: 1, OtherRatio: 1, UsingGroup: "default"}
			u := &Usage{RequestStatus: tc.status, PromptTokens: 100, CompletionTokens: 50, CallType: "chat", TraceID: strings.Repeat("e", 64)}
			if err := PostConsumeQuotaWithPrice(pd, user.ID, key.ID, "captured key", false, 1, "gpt-model", u, 80, "1", 20); err != nil {
				t.Fatal(err)
			}
			logs, total, err := model.ListLogs(user.ID, 1, 10, "")
			if err != nil || total != 1 || logs[0].Status != tc.status || logs[0].ErrorCode != tc.code || logs[0].Quota != 100 || logs[0].PromptTokens != 100 || logs[0].TraceID != u.TraceID || logs[0].BillingDetail == "" {
				t.Fatalf("settled partial outcome: %+v %v", logs, err)
			}
			updated, err := model.GetUserByID(user.ID)
			if err != nil || updated.Quota != 900 {
				t.Fatalf("partial settlement debit: %+v %v", updated, err)
			}
		})
	}
}
