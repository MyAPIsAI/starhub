package relay

import (
	"errors"
	"fmt"

	"starhub/common"
)

var (
	ErrMaxTokensOutOfRange = errors.New("max_tokens exceeds the allowed maximum")
	ErrChoicesOutOfRange   = errors.New("n exceeds the allowed maximum")
)

// ValidateChatRequest rejects billing multiplier fields that are out of
// bounds BEFORE any quota is reserved (the request validation stage): a
// request carrying a huge n / max_tokens combination must be refused with a
// 400 rather than pre-charging an absurd amount or blowing up the settle
// math. The same limits back the pre-charge estimate (see
// CompletionBudgetTokens), so what is validated is exactly what bills.
func ValidateChatRequest(body []byte) error {
	budget := ExtractRequestBudget(body)
	if budget.MaxTokens < 0 || budget.MaxTokens > common.MaxChatMaxTokens {
		return fmt.Errorf("%w: must be between 0 and %d, got %d",
			ErrMaxTokensOutOfRange, common.MaxChatMaxTokens, budget.MaxTokens)
	}
	choices := budget.Choices
	if choices == 0 {
		choices = 1
	}
	if choices < 0 || choices > common.MaxChatChoices {
		return fmt.Errorf("%w: must be between 1 and %d, got %d",
			ErrChoicesOutOfRange, common.MaxChatChoices, budget.Choices)
	}
	return nil
}
