package model

import (
	"testing"
	"time"
)

func TestMoneyBusinessDayFreezesRegionBeforeLaterPolicyChanges(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 100)
	if err := DB.Create(&RegionProfile{ID: "frozen", Name: "Frozen", Timezone: "Asia/Dubai", Enabled: true, FeatureFlags: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("STARHUB_REGION_PROFILE", "frozen")
	if err := IncreaseUserQuota(u.ID, 1); err != nil {
		t.Fatal(err)
	}
	var row MoneyEntry
	if err := DB.Where("user_id = ? AND kind = ?", u.ID, "system_credit").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.BusinessTimezone != "Asia/Dubai" || row.BusinessRegion != "frozen" || row.BusinessDate == "" {
		t.Fatal("funding calendar not frozen", row)
	}
	DB.Model(&RegionProfile{}).Where("id = ?", "frozen").UpdateColumn("timezone", "UTC")
	var after MoneyEntry
	DB.First(&after, "id = ?", row.ID)
	if after.BusinessDaySnapshot != row.BusinessDaySnapshot {
		t.Fatal("historical day changed")
	}
	var work ReconciliationWork
	if err := DB.First(&work, "id = ?", reportKey(u.ID, row.BusinessDaySnapshot)).Error; err != nil || work.EligibleAt <= time.Now().Unix() {
		t.Fatal("funding did not schedule its original day", work, err)
	}
}
