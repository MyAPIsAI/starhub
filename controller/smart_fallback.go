package controller

import (
	"starhub/common"
	"starhub/relay"
)

// nextSmartModel runs only after exhausting the current model's resources and
// before committing a response. Additional quota is reserved before sending.
func (f *relayFlow) nextSmartModel() bool {
	if f.sw.wrote || f.uncertain || f.req.chainAdmission != nil || f.chatIR == nil {
		return false
	}
	for len(f.req.fallbacks) > 0 {
		candidate := f.req.fallbacks[0]
		f.req.fallbacks = f.req.fallbacks[1:]
		prices := f.req.fallbackPricing[candidate.Model]
		group := f.req.group
		if group == common.AutoGroup && len(f.req.autoGroups) > 0 {
			group = f.req.autoGroups[0]
		}
		price, ok := prices[group]
		if !ok {
			continue
		}
		if f.preQuota > 0 {
			estimate := relay.EstimateIRPreConsumeQuotaWithPriceData(&price, f.chatIR)
			if estimate > f.preQuota {
				additional, err := relay.PreConsumeQuotaAmount(f.req.userID, f.req.tokenID, f.req.tokenUnlimited, estimate-f.preQuota, f.req.quotaReservationID)
				if err != nil {
					f.lastErr = err
					continue
				}
				f.preQuota += additional
			}
		}
		f.req.modelName, f.req.body, f.req.channelCheck, f.req.pricing = candidate.Model, candidate.Body, candidate.Check, prices
		f.currentGroup, f.groupIdx = group, 0
		f.tried = map[int64]bool{}
		f.modelFallback = true
		f.span.SetModel(candidate.Model)
		return true
	}
	return false
}
