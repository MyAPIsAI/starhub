package setting

import (
	"encoding/json"
	"fmt"
	"sync"
)

// OptionKeyGroupGroupRatio is the 2D multiplier matrix: user group x using
// group. When a user belonging to group "vip" consumes through a channel in
// group "premium", the final multiplier is group_ratio("premium") *
// group_group_ratio["vip"]["premium"]. A missing entry means 1.0 (no extra
// multiplier). This lets operators give a VIP user a flat discount on every
// group, or upcharge a free-tier user on premium channels.
const OptionKeyGroupGroupRatio = "group_group_ratio"

var (
	groupGroupRatioMu sync.RWMutex
	groupGroupRatio   = map[string]map[string]float64{}
)

// GroupGroupRatioKeys lists the option keys owned by this file.
func GroupGroupRatioKeys() []string {
	return []string{OptionKeyGroupGroupRatio}
}

// IsGroupGroupRatioKey reports whether the key is the group-group ratio
// option.
func IsGroupGroupRatioKey(key string) bool {
	return key == OptionKeyGroupGroupRatio
}

// GroupGroupRatioValue returns the extra multiplier for a user group x using
// group pair; missing entries default to 1.0 (no extra multiplier).
func GroupGroupRatioValue(userGroup, usingGroup string) float64 {
	groupGroupRatioMu.RLock()
	defer groupGroupRatioMu.RUnlock()
	if inner, ok := groupGroupRatio[userGroup]; ok {
		if v, ok := inner[usingGroup]; ok {
			return v
		}
	}
	return 1.0
}

// UpdateGroupGroupRatio parses and applies the 2D matrix option.
func UpdateGroupGroupRatio(value string) error {
	m, err := parseGroupGroupRatio(value)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", OptionKeyGroupGroupRatio, err)
	}
	groupGroupRatioMu.Lock()
	groupGroupRatio = m
	groupGroupRatioMu.Unlock()
	return nil
}

// SnapshotGroupGroupRatio returns the persisted representation of the matrix.
func SnapshotGroupGroupRatio() (string, error) {
	groupGroupRatioMu.RLock()
	defer groupGroupRatioMu.RUnlock()
	b, err := json.Marshal(groupGroupRatio)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseGroupGroupRatio(value string) (map[string]map[string]float64, error) {
	var m map[string]map[string]float64
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		return nil, err
	}
	for userGroup, inner := range m {
		if userGroup == "" {
			return nil, fmt.Errorf("empty user group key")
		}
		for usingGroup, v := range inner {
			if usingGroup == "" {
				return nil, fmt.Errorf("empty using group key")
			}
			if v < 0 || v != v || v > 1e9 {
				return nil, fmt.Errorf("invalid ratio for %s/%s: %v", userGroup, usingGroup, v)
			}
		}
	}
	return m, nil
}

// ResetGroupGroupRatioToDefaults restores the empty default matrix.
func ResetGroupGroupRatioToDefaults() {
	groupGroupRatioMu.Lock()
	defer groupGroupRatioMu.Unlock()
	groupGroupRatio = map[string]map[string]float64{}
}
