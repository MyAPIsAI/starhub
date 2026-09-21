package relay

import "starhub/relay/dto"

// The canonical protocol types live in relay/dto (P3.2's unified
// intermediate representation). The aliases below keep the historical
// relay.* references working across the billing core and the tests.
type (
	Usage                   = dto.Usage
	PromptTokensDetails     = dto.PromptTokensDetails
	CompletionTokensDetails = dto.CompletionTokensDetails
)

// ContentText extracts the text of an OpenAI message content field
// (string or parts array); nil-safe for empty raw messages.
func ContentText(raw []byte) string { return dto.ContentText(raw) }
