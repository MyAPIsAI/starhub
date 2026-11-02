package common

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var groupMultiplierPattern = regexp.MustCompile(`^\+?([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?$`)

// ParseServiceGroupMultiplier checks exact decimal precision before converting
// to the serving ratio. Bounded exponents keep rational parsing inexpensive.
func ParseServiceGroupMultiplier(value string) (float64, error) {
	invalid := errors.New("multiplier must be between 0 and 100 with at most four decimal places")
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return 0, invalid
	}
	match := groupMultiplierPattern.FindStringSubmatch(value)
	if match == nil {
		return 0, invalid
	}
	if match[2] != "" {
		exponent, err := strconv.Atoi(match[2])
		if err != nil || exponent < -1000 || exponent > 1000 {
			return 0, invalid
		}
	}
	ratio, ok := new(big.Rat).SetString(value)
	if !ok || ratio.Sign() < 0 || ratio.Cmp(big.NewRat(100, 1)) > 0 {
		return 0, invalid
	}
	scaled := new(big.Rat).Mul(ratio, big.NewRat(10000, 1))
	if !scaled.IsInt() {
		return 0, invalid
	}
	return float64(scaled.Num().Int64()) / 10000, nil
}
