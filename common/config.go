package common

const (
	RoleUser  = 1
	RoleAdmin = 10
	RoleRoot  = 100
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

const QuotaPerUSD = 500000

const (
	DefaultGroup         = "default"
	DefaultTokenQuota    = 500000
	DefaultRootUser      = "root"
	DefaultRootPasswd    = "123456"
	DefaultChannelType   = 1
	DefaultRegisterQuota = 0
)

const Version = "0.1.0"
