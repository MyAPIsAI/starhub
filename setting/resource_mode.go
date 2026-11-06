package setting

import (
	"fmt"
	"sync"
)

// Resource provider mode option key.
const OptionKeyResourceProviderMode = "resource_provider_mode"

const (
	ResourceProviderModeLocal      = "local"
	ResourceProviderModeCommercial = "commercial"
)

var (
	resourceModeMu       sync.RWMutex
	resourceProviderMode = ResourceProviderModeLocal
)

// ResourceProviderModeKeys lists the option keys owned by this file.
func ResourceProviderModeKeys() []string {
	return []string{OptionKeyResourceProviderMode}
}

// IsResourceProviderModeKey reports whether the key is a resource mode option.
func IsResourceProviderModeKey(key string) bool {
	return key == OptionKeyResourceProviderMode
}

// ResourceProviderModeValue returns the current resource provider mode.
func ResourceProviderModeValue() string {
	resourceModeMu.RLock()
	defer resourceModeMu.RUnlock()
	return resourceProviderMode
}

// IsCommercialMode reports whether the system is in commercial resource mode.
func IsCommercialMode() bool {
	return ResourceProviderModeValue() == ResourceProviderModeCommercial
}

// UpdateResourceProviderMode applies a resource mode option.
func UpdateResourceProviderMode(key, value string) error {
	if key != OptionKeyResourceProviderMode {
		return fmt.Errorf("unknown resource mode option key: %s", key)
	}
	switch value {
	case ResourceProviderModeLocal, ResourceProviderModeCommercial:
		resourceModeMu.Lock()
		resourceProviderMode = value
		resourceModeMu.Unlock()
		return nil
	default:
		return fmt.Errorf("invalid value for %s: must be 'local' or 'commercial'", key)
	}
}

// SnapshotResourceProviderMode returns the persisted representation.
func SnapshotResourceProviderMode(key string) (string, error) {
	if key != OptionKeyResourceProviderMode {
		return "", fmt.Errorf("unknown resource mode option key: %s", key)
	}
	resourceModeMu.RLock()
	defer resourceModeMu.RUnlock()
	return resourceProviderMode, nil
}

// ResetResourceProviderModeToDefault restores the compiled-in default.
func ResetResourceProviderModeToDefault() {
	resourceModeMu.Lock()
	defer resourceModeMu.Unlock()
	resourceProviderMode = ResourceProviderModeLocal
}
