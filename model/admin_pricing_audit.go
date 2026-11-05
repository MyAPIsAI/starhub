package model

import (
	"bytes"
	"encoding/json"
	"gorm.io/gorm"
	"sort"
	"starhub/setting"
)

func recordPricingChange(tx *gorm.DB, key, before, after string, actor int64) error {
	if actor <= 0 || (!isPricingOption(key) && key != setting.OptionKeyGroupRatio && key != "model_time_pricing" && key != "model_time_token_prices") {
		return nil
	}
	var old, new map[string]json.RawMessage
	if json.Unmarshal([]byte(before), &old) != nil || json.Unmarshal([]byte(after), &new) != nil {
		return RecordAdminChange(tx, actor, "pricing_changed", "pricing", key, map[string]string{"value": before}, map[string]string{"value": after}, "", "")
	}
	names := map[string]bool{}
	for name := range old {
		names[name] = true
	}
	for name := range new {
		names[name] = true
	}
	ordered := []string{}
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		if bytes.Equal(old[name], new[name]) {
			continue
		}
		action, kind := "model_pricing_changed", "models"
		if key == setting.OptionKeyGroupRatio {
			action = "service_group_multiplier_changed"
			kind = "service-group"
		}
		var oldValue, newValue any
		if len(old[name]) > 0 {
			_ = json.Unmarshal(old[name], &oldValue)
		}
		if len(new[name]) > 0 {
			_ = json.Unmarshal(new[name], &newValue)
		}
		if err := RecordAdminChange(tx, actor, action, kind, name, map[string]any{"component": key, "value": oldValue}, map[string]any{"component": key, "value": newValue}, "", ""); err != nil {
			return err
		}
	}
	return nil
}
