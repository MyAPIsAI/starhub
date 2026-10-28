package relay

import (
	"starhub/model"
)

func settleDurableQuota(userID, tokenID int64, unlimited bool, delta, actual int64, entry *model.Log) error {
	intent, err := model.RecordQuotaSettlement(userID, tokenID, unlimited, delta, actual, entry)
	if err != nil {
		return err
	}
	settled, claimed, err := model.ApplyQuotaSettlement(intent.ID)
	if err != nil {
		return err
	}
	if claimed {
		*entry = *settled
		applyAffiliateRebate(userID, entry.Quota)
	}
	return nil
}

// RetryQuotaSettlement applies a frozen bill without another upstream call or
// a pricing lookup. Concurrent foreground/background workers claim it once.
func RetryQuotaSettlement(id int64) error {
	entry, claimed, err := model.ApplyQuotaSettlement(id)
	if err == nil && claimed {
		applyAffiliateRebate(entry.UserID, entry.Quota)
	}
	return err
}
