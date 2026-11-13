package common

import (
	"errors"
	"math/big"
	"regexp"
)

const NanoPerUSD int64 = 1_000_000_000
const NanoPerLegacyQuota int64 = 2000
const MoneyUnitNano = "usd_nano"
const MaxNanoPerRequest = MaxQuotaPerRequest * NanoPerLegacyQuota

var nanoInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var usdDecimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,9})?$`)

// ParseNano preserves integers beyond JavaScript's exact numeric range. Amount
// strings are canonical and bounded before allocating arbitrary-precision data.
func ParseNano(value string) (*big.Int, error) {
	if len(value) > 64 || !nanoInteger.MatchString(value) || value == "-0" {
		return nil, errors.New("invalid nano-USD amount")
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, errors.New("invalid nano-USD amount")
	}
	return amount, nil
}

func ParseUSDNano(value string) (*big.Int, error) {
	if len(value) > 64 || !usdDecimal.MatchString(value) {
		return nil, errors.New("invalid USD amount")
	}
	usd, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, errors.New("invalid USD amount")
	}
	usd.Mul(usd, new(big.Rat).SetInt64(NanoPerUSD))
	if !usd.IsInt() {
		return nil, errors.New("USD amount exceeds nano precision")
	}
	return new(big.Int).Set(usd.Num()), nil
}

// QuotaMoney is an exact compatibility representation: legacy whole ticks plus
// a nano remainder. Existing balances need no destructive scaling migration.
func QuotaMoney(quota, remainder int64) *big.Int {
	return new(big.Int).Add(new(big.Int).Mul(big.NewInt(quota), big.NewInt(NanoPerLegacyQuota)), big.NewInt(remainder))
}

func NanoQuotaParts(amount *big.Int) (int64, int64, error) {
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(amount, big.NewInt(NanoPerLegacyQuota), remainder)
	if !whole.IsInt64() {
		return 0, 0, errors.New("monetary balance exceeds storage bounds")
	}
	return whole.Int64(), remainder.Int64(), nil
}
