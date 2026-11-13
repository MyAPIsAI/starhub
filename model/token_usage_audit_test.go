package model

import "testing"

func TestTokenAuditDetectsOneNanoWithoutChangingCounters(t *testing.T) {
	newTestDB(t)
	u, token := newBillingUser(t, 1000)
	for _, id := range []string{"n2:audit-one", "n2:audit-two"} {
		if err := ReserveQuota(u.ID, token.ID, false, 100, id); err != nil {
			t.Fatal(err)
		}
		if err := SettleQuota(u.ID, token.ID, false, -99, 1, &Log{QuotaReservationID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AuditMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&Token{}).Where("id = ?", token.ID).UpdateColumn("used_quota_remainder_nano", 3).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AuditMoneyLedger(u.ID); err == nil {
		t.Fatal("one-nano key drift was accepted")
	}
	var after Token
	DB.First(&after, token.ID)
	if after.UsedQuotaRemainderNano != 3 {
		t.Fatal("audit repaired counters")
	}
}

func TestTokenPeriodAuditDetectsHistoricalPeriodDrift(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1)
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:period-audit"); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -99, 1, &Log{QuotaReservationID: "n2:period-audit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&TokenQuotaPeriod{}).Where("id = ?", token.QuotaPeriodID).UpdateColumn("used_quota_remainder_nano", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AuditMoneyLedger(u.ID); err == nil {
		t.Fatal("period drift was accepted")
	}
}
