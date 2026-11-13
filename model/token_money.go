package model

import (
	"errors"
	"starhub/common"
)

// NormalizeTokenMoneyFields converts canonical public strings into the exact
// compatibility projection. A request cannot specify both monetary versions.
func NormalizeTokenMoneyFields(fields map[string]any) error {
	for _, key := range []string{"remain_quota", "period_quota_limit"} {
		raw, found := fields[key+"_nano"]
		if !found {
			continue
		}
		if _, ambiguous := fields[key]; ambiguous {
			return errors.New("conflicting quota units")
		}
		text, ok := raw.(string)
		if !ok {
			return errors.New("nano quota must be a canonical integer string")
		}
		amount, err := common.ParseNano(text)
		if err != nil || amount.Sign() < 0 {
			return errors.New("invalid nano quota")
		}
		whole, remainder, err := common.NanoQuotaParts(amount)
		if err != nil {
			return err
		}
		fields[key], fields[key+"_remainder_nano"] = whole, remainder
		delete(fields, key+"_nano")
	}
	return nil
}
