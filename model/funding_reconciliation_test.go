package model

import (
	"testing"

	"starhub/common"
)

func TestFundingReconciliationDetectsIndependentOneNanoSourceDifference(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 100)
	if err := IncreaseUserQuota(u.ID, 1); err != nil {
		t.Fatal(err)
	}
	var receipt FundingReceipt
	if err := DB.Where("user_id = ?", u.ID).First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	report, _, err := ReconcileBusinessDay(u.ID, receipt.BusinessDaySnapshot)
	if err != nil || report.State != "matched" || report.FundingCreditsNano != common.QuotaMoney(1, 0).String() || report.FundingDifferenceNano != "0" {
		t.Fatal(report, err)
	}
	if err := DB.Model(&receipt).UpdateColumn("amount_nano", common.QuotaMoney(1, 1).String()).Error; err != nil {
		t.Fatal(err)
	}
	report, changed, err := ReconcileBusinessDay(u.ID, receipt.BusinessDaySnapshot)
	if err != nil || !changed || report.State != "difference" || report.FundingDifferenceNano != "1" {
		t.Fatal("independent funding mismatch hidden", report, err)
	}
	after, _ := GetUserByID(u.ID)
	if after.Quota != 101 || after.QuotaRemainderNano != 0 {
		t.Fatal("reconciliation changed funds")
	}
	DB.Delete(&receipt)
	report, _, err = ReconcileBusinessDay(u.ID, receipt.BusinessDaySnapshot)
	if err != nil || report.UnverifiableFunding != 1 || report.State != "difference" {
		t.Fatal("missing funding evidence fabricated", report, err)
	}
	if err := DB.Model(&MoneyEntry{}).Where("user_id = ? AND kind = ?", u.ID, "system_credit").Updates(map[string]any{"business_date": "", "business_calendar_version": 0, "business_timezone": "", "business_region": "", "business_start": 0, "business_end": 0}).Error; err != nil {
		t.Fatal(err)
	}
	legacy := legacyBusinessDay(receipt.CreatedAt)
	report, _, err = ReconcileBusinessDay(u.ID, legacy)
	if err != nil || report.State != "incomplete" || report.UnverifiableFunding != 1 || report.FundingDifferenceNano != "0" {
		t.Fatal("legacy missing evidence reported as proven money difference", report, err)
	}
}
