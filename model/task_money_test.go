package model

import (
	"encoding/json"
	"testing"
)

func TestTaskMoneyJSONAndPendingTotalsRespectStoredMonetaryVersions(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 1000)
	for _, task := range []Task{{UserID: user.ID, TokenID: token.ID, Status: "submitted", Quota: 1}, {UserID: user.ID, TokenID: token.ID, Status: "submitted", Quota: 1, QuotaReservationID: "n2:task-money"}} {
		if err := DB.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		want := "2000"
		if task.QuotaReservationID != "" {
			want = "1"
		}
		if value["quota_nano"] != want || value["amount_unit"] != "usd_nano" {
			t.Fatal("task JSON lost monetary version")
		}
	}
	amount, err := SumPendingTaskMoney(DB, token.ID)
	if err != nil || amount.String() != "2001" {
		t.Fatalf("mixed-unit tasks were summed as one scale: %v %v", amount, err)
	}
	whole, err := SumPendingTaskQuota(DB, token.ID)
	if err != nil || whole != 1 {
		t.Fatalf("legacy display projection changed scale: %d %v", whole, err)
	}
}
