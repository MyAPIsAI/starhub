package service

import (
	"context"
	"log"

	"starhub/model"
	"starhub/relay"
)

// RetryPendingQuotaSettlementsOnce resumes durable synchronous bills.
func RetryPendingQuotaSettlementsOnce(ctx context.Context) {
	intents, err := model.ListPendingQuotaSettlements(200)
	if err != nil {
		log.Printf("list pending quota settlements failed: %v", err)
		return
	}
	for _, intent := range intents {
		if ctx.Err() != nil {
			return
		}
		if err := relay.RetryQuotaSettlement(intent.ID); err != nil {
			log.Printf("quota settlement %d failed: %v", intent.ID, err)
		}
	}
	RetryPendingAffiliateRebatesOnce(ctx)
}

func RetryPendingAffiliateRebatesOnce(ctx context.Context) {
	items, err := model.ListPendingAffiliateRebates(200)
	if err != nil {
		log.Printf("list pending affiliate rebates failed: %v", err)
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		if err := model.DeliverAffiliateRebate(item.LogID); err != nil {
			log.Printf("affiliate rebate for log %d failed: %v", item.LogID, err)
		}
	}
}
