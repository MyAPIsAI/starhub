package model

import (
	"context"
	"math"
	"math/big"
	"starhub/common"
	"testing"
	"time"
)

func TestUsageMoneyAggregatesKeepFractionalCarriesAndLargeTotals(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 1000)
	now := time.Now().UTC()
	for _, remainder := range []int64{1, 1999} {
		entry := Log{UserID: user.ID, TokenID: token.ID, TokenName: token.Name, Model: "fractional", QuotaRemainderNano: remainder, UnfundedQuotaRemainderNano: remainder, Status: 1, CreatedAt: now.Unix()}
		if err := DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	daily, err := SummarizeLogsByDay(user.ID, now.Unix()-60)
	if err != nil || len(daily) != 1 || daily[0].QuotaNano != "2000" || daily[0].UnfundedQuotaNano != "2000" || daily[0].Quota != 1 {
		t.Fatalf("fractional carry lost: %+v %v", daily, err)
	}
	models, err := SummarizeLogsByModel(user.ID, now.Unix()-60)
	if err != nil || len(models) != 1 || models[0].QuotaNano != "2000" {
		t.Fatalf("model totals: %+v %v", models, err)
	}
	keys, err := SummarizeLogsByToken(user.ID, token.ID, now.Unix()-60)
	if err != nil || len(keys) != 1 || keys[0].QuotaNano != "2000" {
		t.Fatalf("key totals: %+v %v", keys, err)
	}
	for range 2 {
		if err := DB.Create(&Log{UserID: user.ID, TokenID: token.ID, Model: "large", Quota: math.MaxInt64, QuotaRemainderNano: 1999, Status: 1, CreatedAt: now.Unix()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	expected := common.QuotaMoney(math.MaxInt64, 1999)
	expected.Mul(expected, big.NewInt(2)).Add(expected, big.NewInt(2000))
	daily, err = SummarizeLogsByDay(user.ID, now.Unix()-60)
	if err != nil || daily[0].QuotaNano != expected.String() || daily[0].Quota != math.MaxInt64 {
		t.Fatalf("exact total overflowed: %+v %v", daily, err)
	}
	quality, err := SummarizeServiceQuality(user.ID, 1)
	if err != nil || quality.Overall.TotalQuotaNano != expected.String() {
		t.Fatalf("quality money sum lost precision: %+v %v", quality, err)
	}
	finance, err := ReadFinanceSummary(context.Background(), now.Unix()-60, now.Unix()+60)
	if err != nil || finance.APICollectedNano != expected.String() || finance.APIUnfundedNano != "2000" {
		t.Fatalf("finance rounding or overflow: %+v %v", finance, err)
	}
	view, err := ReadOwnedTokenUsage(user.ID, token.ID, 1, 1, now)
	if err != nil || len(view.Models) != 2 || view.Models[0].Model != "large" || view.Trend[29].QuotaNano != expected.String() || view.Trend[0].QuotaNano != "0" {
		t.Fatalf("regional exact totals: %+v %v", view, err)
	}
}

func TestTokenUsageCurrentExposesFractionalLifetimeAndPendingBudgets(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, token.ID, false, 1, "n2:usage-pending"); err != nil {
		t.Fatal(err)
	}
	view, err := ReadOwnedTokenUsage(user.ID, token.ID, 1, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if view.Current.ReservedQuotaNano != "1" || *view.Current.QuotaLimitNano != "2000000" || *view.Current.RemainingQuotaNano != "1999999" || view.Current.UsedQuotaNano != "0" {
		t.Fatalf("pending fraction disappeared: %+v", view.Current)
	}
	if err := SettleQuota(user.ID, token.ID, false, 0, 1, &Log{QuotaReservationID: "n2:usage-pending", AmountUnit: common.MoneyUnitNano}); err != nil {
		t.Fatal(err)
	}
	view, err = ReadOwnedTokenUsage(user.ID, token.ID, 1, 1, time.Now())
	if err != nil || view.Current.LifetimeUsedQuotaNano != "1" || *view.Current.QuotaLimitNano != "2000000" {
		t.Fatalf("lifetime fraction disappeared: %+v %v", view, err)
	}
}
