package model

import "testing"

func TestUnavailableRebateRecipientsDoNotStarveLaterWork(t *testing.T) {
	newTestDB(t)
	for i := int64(1); i <= 3; i++ {
		if err := DB.Create(&AffiliateRebate{LogID: i, UserID: 1, RecipientID: 9999, Quota: 10}).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, err := ListPendingAffiliateRebates(2)
	if err != nil || len(page) != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for _, item := range page {
		if err := DeliverAffiliateRebate(item.LogID); err == nil {
			t.Fatal("missing recipient was acknowledged")
		}
	}
	next, err := ListPendingAffiliateRebates(2)
	if err != nil || len(next) != 2 || next[0].LogID != 3 {
		t.Fatalf("failed recipients starved later work: %+v %v", next, err)
	}
	var remaining int64
	if err := DB.Model(&AffiliateRebate{}).Where("completed_at = 0").Count(&remaining).Error; err != nil || remaining != 3 {
		t.Fatalf("missing-recipient rebate lost: count=%d err=%v", remaining, err)
	}
}
