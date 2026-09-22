package setting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// Affiliate / invite rebate settings (P4.4 commercialization):
//
//	aff_rebate_ratio   0.1   inviter gets 10% of every invitee's spend
//	aff_register_bonus 0     one-time quota bonus credited to the inviter
//	                        when an invitee registers (0 = no bonus)
const (
	OptionKeyAffRebateRatio   = "aff_rebate_ratio"
	OptionKeyAffRegisterBonus = "aff_register_bonus"
)

var (
	affMu            sync.RWMutex
	affRebateRatio   = 0.1
	affRegisterBonus = int64(0)
)

// AffiliateOptionKeys lists the option keys owned by this file.
func AffiliateOptionKeys() []string {
	return []string{OptionKeyAffRebateRatio, OptionKeyAffRegisterBonus}
}

// IsAffiliateOptionKey reports whether the key is an affiliate option.
func IsAffiliateOptionKey(key string) bool {
	switch key {
	case OptionKeyAffRebateRatio, OptionKeyAffRegisterBonus:
		return true
	}
	return false
}

// AffRebateRatioValue returns the inviter rebate ratio (0 = no rebate).
func AffRebateRatioValue() float64 {
	affMu.RLock()
	defer affMu.RUnlock()
	return affRebateRatio
}

// AffRegisterBonusValue returns the one-time invite bonus in quota.
func AffRegisterBonusValue() int64 {
	affMu.RLock()
	defer affMu.RUnlock()
	return affRegisterBonus
}

// UpdateAffiliate applies an affiliate option key.
func UpdateAffiliate(key, value string) error {
	switch key {
	case OptionKeyAffRebateRatio:
		var v float64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v < 0 || v != v || v > 1 {
			return fmt.Errorf("invalid value for %s: must be in [0,1]", key)
		}
		affMu.Lock()
		affRebateRatio = v
		affMu.Unlock()
		return nil
	case OptionKeyAffRegisterBonus:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v < 0 {
			return fmt.Errorf("invalid value for %s: must be >= 0", key)
		}
		affMu.Lock()
		affRegisterBonus = v
		affMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown affiliate option key: %s", key)
	}
}

// SnapshotAffiliate returns the persisted representation of a key.
func SnapshotAffiliate(key string) (string, error) {
	affMu.RLock()
	defer affMu.RUnlock()
	switch key {
	case OptionKeyAffRebateRatio:
		return strconv.FormatFloat(affRebateRatio, 'f', -1, 64), nil
	case OptionKeyAffRegisterBonus:
		return strconv.FormatInt(affRegisterBonus, 10), nil
	default:
		return "", fmt.Errorf("unknown affiliate option key: %s", key)
	}
}

// ResetAffiliateToDefaults restores the compiled-in affiliate defaults.
func ResetAffiliateToDefaults() {
	affMu.Lock()
	defer affMu.Unlock()
	affRebateRatio = 0.1
	affRegisterBonus = 0
}