package service

import (
	"math"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/setting"
)

func TestShadowBillingPreservesFractionalFeesAndMissingHistory(t *testing.T) {
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	price := &relay.PriceData{ModelName: "nano-shadow", PriceVersion: 19, AmountUnit: common.MoneyUnitNano, ModelPriceUSD: 0.000000001, HasModelPrice: true, GroupRatio: 1, OtherRatio: 1, UsingGroup: "vip"}
	insert := func(charged, unfunded int64) {
		t.Helper()
		raw, err := relay.EncodeTokenBillingEvidence(price, &relay.Usage{})
		if err != nil {
			t.Fatal(err)
		}
		if err := model.InsertLog(&model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit, PriceVersion: 19, BillingEvidence: raw, QuotaRemainderNano: charged, UnfundedQuotaRemainderNano: unfunded}); err != nil {
			t.Fatal(err)
		}
	}
	insert(1, 0)
	insert(2, 0)
	price.ModelPriceUSD = 0.000000002
	insert(1, 1)
	if err := model.InsertLog(&model.Log{Model: "legacy-shadow", PromptTokens: 10, Quota: 500}); err != nil {
		t.Fatal(err)
	}
	if err := setting.ApplyVersionedOptions(map[string]string{"model_price": `{"nano-shadow":999}`, "group_ratio": `{"default":100}`}, 20); err != nil {
		t.Fatal(err)
	}
	report, err := RunShadowBillingVerification(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalRecords != 4 || report.VerifiedRecords != 3 || report.UnverifiableRecords != 1 || report.RecordsWithDiff != 1 || report.TotalDiffNano != "-1" || report.TotalAbsoluteDiffNano != "1" {
		t.Fatalf("incorrect fractional verification: %+v", report)
	}
	for _, entry := range report.Entries {
		if entry.Status == "difference" && (entry.StoredNano != "2" || entry.RecomputedNano != "1" || entry.Group != "vip" || entry.PriceVersion != 19) {
			t.Fatalf("lost frozen facts: %+v", entry)
		}
	}
}

func TestShadowBillingAggregatesDifferencesBeyondInt64(t *testing.T) {
	newTestDB(t)
	price := &relay.PriceData{ModelName: "large-shadow", PriceVersion: 29, AmountUnit: common.MoneyUnitNano, ModelPriceUSD: 0.000000001, HasModelPrice: true, GroupRatio: 1, OtherRatio: 1}
	raw, err := relay.EncodeTokenBillingEvidence(price, &relay.Usage{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := model.InsertLog(&model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit, PriceVersion: 29, BillingEvidence: raw, Quota: math.MaxInt64}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := RunShadowBillingVerification(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.RecordsWithDiff != 2 || report.TotalDiffNano != "-36893488147419103227998" || report.TotalAbsoluteDiffNano != "36893488147419103227998" || report.TotalDiff != math.MinInt64 {
		t.Fatalf("difference aggregation overflowed: %+v", report)
	}
}

func TestRunShadowBillingVerificationWithSyntheticData(t *testing.T) {
	newTestDB(t)
	// Seed: insert logs with known token counts and quota.
	user := &model.User{Username: "shadow-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	// Insert 3 logs: one matching default pricing, one with diff, one no-tokens (skipped).
	price := &relay.PriceData{ModelName: "gpt-4o", PriceVersion: 7, ModelPriceUSD: 0.003, HasModelPrice: true, GroupRatio: 1, OtherRatio: 1}
	for _, input := range []struct{ prompt, completion, stored int64 }{{1000, 500, 1500}, {2000, 1000, 100}} {
		evidence, err := relay.EncodeTokenBillingEvidence(price, &relay.Usage{PromptTokens: input.prompt, CompletionTokens: input.completion})
		if err != nil {
			t.Fatal(err)
		}
		if err := model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", PromptTokens: input.prompt, CompletionTokens: input.completion, Quota: input.stored, PriceVersion: 7, BillingEvidence: evidence, Status: 1, ChannelID: 1, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", PromptTokens: 0, CompletionTokens: 0, Quota: 0, Status: 1, ChannelID: 1, CreatedAt: now})

	report, err := RunShadowBillingVerification(now-3600, 100)
	if err != nil {
		t.Fatalf("RunShadowBillingVerification failed: %v", err)
	}
	if report.TotalRecords != 2 {
		t.Fatalf("expected 2 records (skip zero-token), got %d", report.TotalRecords)
	}
	// The second log has a deliberately wrong quota (100 vs recomputed),
	// so at least 1 record should have a diff.
	if report.RecordsWithDiff == 0 {
		t.Fatal("expected at least 1 record with diff")
	}
}

func TestRunShadowBillingVerificationNoData(t *testing.T) {
	newTestDB(t)
	report, err := RunShadowBillingVerification(time.Now().Add(-1*time.Hour).Unix(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalRecords != 0 {
		t.Fatalf("expected 0 records with empty DB, got %d", report.TotalRecords)
	}
}

func newTestDB(t *testing.T) {
	t.Helper()
	if err := model.InitDB(t.TempDir() + "/shadow.db"); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
}
