package model

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

var ErrRegionUnavailable = errors.New("active region profile is unavailable")

func ActiveRegionID() string {
	if id := strings.TrimSpace(os.Getenv("STARHUB_REGION_PROFILE")); id != "" {
		return id
	}
	return "default"
}

// ActiveRegionProfile is deployment-selected, never chosen by customer headers.
// An absent default preserves legacy deployments; an explicitly selected or
// disabled profile fails closed. Saved quota periods retain their own timezone.
func ActiveRegionProfile() (*RegionProfile, error) {
	profile, err := GetRegionProfile(ActiveRegionID())
	if errors.Is(err, ErrRegionProfileNotFound) && os.Getenv("STARHUB_REGION_PROFILE") == "" {
		return &RegionProfile{DefaultLanguage: "en", SupportedLanguages: "zh,en,ar,ko", Currency: "USD", Timezone: "UTC", FeatureFlags: "{}", Enabled: true}, nil
	}
	if err != nil || !profile.Enabled {
		return nil, ErrRegionUnavailable
	}
	if _, err := time.LoadLocation(profile.Timezone); err != nil {
		return nil, ErrRegionUnavailable
	}
	var flags map[string]bool
	if json.Unmarshal([]byte(profile.FeatureFlags), &flags) != nil || flags == nil {
		return nil, ErrRegionUnavailable
	}
	return profile, nil
}

func (profile *RegionProfile) FeatureEnabled(name string) bool {
	var flags map[string]bool
	if json.Unmarshal([]byte(profile.FeatureFlags), &flags) != nil {
		return false
	}
	enabled, exists := flags[name]
	return !exists || enabled
}

func (profile *RegionProfile) AuthEnabled(name string) bool {
	if profile.AuthPlugins == "" {
		return true
	}
	for _, value := range SplitList(profile.AuthPlugins) {
		if value == name {
			return true
		}
	}
	return false
}

func (profile *RegionProfile) PaymentEnabled(name string) bool {
	if !profile.FeatureEnabled("payments") {
		return false
	}
	if profile.PaymentPlugins == "" {
		return true
	}
	for _, value := range SplitList(profile.PaymentPlugins) {
		if value == name {
			return true
		}
	}
	return false
}
