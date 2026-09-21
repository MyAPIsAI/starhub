package relay

import (
	"errors"
	"math"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

var (
	ErrInsufficientTokenQuota = errors.New("token quota is not enough to process this request")
	ErrInsufficientUserQuota  = errors.New("user quota is not enough to process this request")
	ErrUserMissing            = errors.New("relay user not found")
	ErrTokenMissing           = errors.New("relay token not found")
)

const estimateReserveTokens = 100

func ComputeQuota(modelName, group string, promptTokens, completionTokens int64) int64 {
	inputPrice := setting.ModelRatioValue(modelName)
	completionRatio := setting.CompletionRatioValue(modelName)
	groupRatio, ok := setting.GroupRatioValue(group)
	if !ok || groupRatio <= 0 {
		groupRatio = 1
	}
	cost := (float64(promptTokens)*inputPrice +
		float64(completionTokens)*inputPrice*completionRatio) /
		1e6 * float64(common.QuotaPerUSD) * groupRatio
	quota := int64(math.Round(cost))
	if quota < 0 {
		quota = 0
	}
	if quota == 0 && (promptTokens > 0 || completionTokens > 0) {
		quota = 1
	}
	return quota
}

func PreConsumeQuota(userID, tokenID int64, tokenUnlimited bool, modelName, group string, body []byte) (int64, error) {
	estimated := EstimateTokens(ExtractPromptText(body)) + estimateReserveTokens
	pre := ComputeQuota(modelName, group, estimated, 0)
	if pre < 1 {
		pre = 1
	}
	user, err := model.GetUserByID(userID)
	if err != nil {
		return 0, ErrUserMissing
	}
	if user.Quota < pre {
		return 0, ErrInsufficientUserQuota
	}
	if !tokenUnlimited {
		token, err := model.GetTokenByID(tokenID)
		if err != nil {
			return 0, ErrTokenMissing
		}
		if token.RemainQuota < pre {
			return 0, ErrInsufficientTokenQuota
		}
		ok, err := model.DecreaseTokenQuota(tokenID, pre)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrInsufficientTokenQuota
		}
	}
	ok, err := model.DecreaseUserQuota(userID, pre)
	if err != nil {
		return 0, err
	}
	if !ok {
		if !tokenUnlimited {
			_ = model.IncreaseTokenQuota(tokenID, pre)
		}
		return 0, ErrInsufficientUserQuota
	}
	return pre, nil
}

func RefundPreConsumedQuota(tokenID, userID int64, tokenUnlimited bool, amount int64) error {
	if amount <= 0 {
		return nil
	}
	if !tokenUnlimited {
		if err := model.IncreaseTokenQuota(tokenID, amount); err != nil {
			return err
		}
	}
	return model.IncreaseUserQuota(userID, amount)
}

func PostConsumeQuota(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName, group string, usage *Usage, preQuota int64) error {
	if usage == nil {
		usage = &Usage{}
	}
	actual := ComputeQuota(modelName, group, usage.PromptTokens, usage.CompletionTokens)
	delta := actual - preQuota
	if delta > 0 {
		if !tokenUnlimited {
			if err := model.DecreaseTokenQuotaUnconditional(tokenID, delta); err != nil {
				return err
			}
		}
		if err := model.DecreaseUserQuotaUnconditional(userID, delta); err != nil {
			return err
		}
	} else if delta < 0 {
		if err := RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, -delta); err != nil {
			return err
		}
	}
	if err := model.IncreaseChannelUsedQuota(channelID, actual); err != nil {
		return err
	}
	if err := model.IncreaseTokenUsedQuota(tokenID, actual); err != nil {
		return err
	}
	if err := model.IncreaseUserUsedQuota(userID, actual); err != nil {
		return err
	}
	return model.InsertLog(&model.Log{
		UserID:           userID,
		TokenID:          tokenID,
		TokenName:        tokenName,
		ChannelID:        channelID,
		Model:            modelName,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		Quota:            actual,
	})
}
