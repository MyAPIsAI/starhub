package relay

import (
	"context"
	"time"

	"starhub/model"
	"starhub/observability"
)

func PreConsumeQuotaObserved(ctx context.Context, userID, tokenID int64, unlimited bool, amount int64) (int64, error) {
	end := observability.Begin(ctx, "precharge", 0)
	quota, err := PreConsumeQuotaAmount(userID, tokenID, unlimited, amount)
	status, code := "success", ""
	if err != nil {
		status, code = "failed", "precharge_failed"
	}
	end(status, code)
	if observability.ID(ctx) != "" {
		_ = model.RecordObservation("precharge", "gateway", err == nil, time.Now())
	}
	return quota, err
}

func RefundQuotaObserved(ctx context.Context, tokenID, userID int64, unlimited bool, amount int64) error {
	end := observability.Begin(ctx, "refund", 0)
	err := RefundPreConsumedQuota(tokenID, userID, unlimited, amount)
	status, code := "success", ""
	if err != nil {
		status, code = "failed", "refund_failed"
		_ = model.RecordObservation("settlement", "refund", false, time.Now())
	}
	end(status, code)
	return err
}
