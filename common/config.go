package common

const (
	RoleUser    = 1
	RoleFinance = 5
	RoleAdmin   = 10
	RoleRoot    = 100
)

const (
	StatusEnabled  = 1
	StatusDisabled = 2
)

const (
	ChannelStatusEnabled          = 1
	ChannelStatusManuallyDisabled = 2
	ChannelStatusAutoDisabled     = 3
)

// RetryTimes is the number of extra attempts (on top of the first one) Relay
// makes when an upstream channel fails with a retryable error (5xx, timeout,
// network). 4xx client errors terminate immediately.
const RetryTimes = 3

const QuotaPerUSD = 500000

// Request-side billing multiplier caps. The relay validates these BEFORE any
// quota is reserved: a hostile n / max_tokens combination must not be able to
// inflate the pre-charge estimate (or the settle math) unboundedly.
const (
	// MaxChatChoices caps the "n" (choices) multiplier of a chat request.
	MaxChatChoices = 10
	// MaxChatMaxTokens caps "max_tokens" / "max_completion_tokens".
	MaxChatMaxTokens = 100000
	// DefaultCompletionBudgetTokens is the completion budget assumed when a
	// request leaves max_tokens to the model; the pre-charge uses it to stay
	// conservative (over-reserve, settle down with real usage).
	DefaultCompletionBudgetTokens = 1024
)

const (
	DefaultGroup         = "default"
	DefaultTokenQuota    = 500000
	DefaultRootUser      = "root"
	DefaultRootPasswd    = "123456"
	DefaultChannelType   = 1
	DefaultRegisterQuota = 0
)

// AutoGroup is the reserved group name that triggers cross-group retry: the
// relay walks the token's AutoGroups sequence instead of a single group.
const AutoGroup = "auto"

const Version = "0.1.0"
