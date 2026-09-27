package setting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Channel affinity (sticky routing) option keys (stored in the option table,
// hot-updatable):
//
//	routing_affinity_enabled          false     master switch
//	routing_affinity_ttl_seconds      300       how long a preference survives
//	routing_affinity_mode             sticky    sticky | hash
//	routing_affinity_cooldown_seconds 30        how long to suppress a failed channel
//	routing_affinity_prefix_cache_enabled  false  enable prompt-prefix scoping
const (
	OptionKeyRoutingAffinityEnabled     = "routing_affinity_enabled"
	OptionKeyRoutingAffinityTTL         = "routing_affinity_ttl_seconds"
	OptionKeyRoutingAffinityMode        = "routing_affinity_mode"
	OptionKeyRoutingAffinityCooldown    = "routing_affinity_cooldown_seconds"
	OptionKeyRoutingAffinityPrefixCache = "routing_affinity_prefix_cache_enabled"
)

const (
	AffinityModeSticky = "sticky"
	AffinityModeHash   = "hash"

	DefaultRoutingAffinityTTL    = 5 * time.Minute
	MaxRoutingAffinityTTL        = 24 * time.Hour
	MaxRoutingAffinityTTLSeconds = int64(24 * 60 * 60)

	DefaultRoutingAffinityCooldown    = 30 * time.Second
	MaxRoutingAffinityCooldown        = 10 * time.Minute
	MaxRoutingAffinityCooldownSeconds = int64(10 * 60)
)

var (
	routeAffMu                 sync.RWMutex
	routingAffinityEnabled     = false
	routingAffinityTTL         = DefaultRoutingAffinityTTL
	routingAffinityMode        = AffinityModeSticky
	routingAffinityCooldown    = DefaultRoutingAffinityCooldown
	routingAffinityPrefixCache = false
)

// AffinityOptionKeys lists the option keys owned by this file.
func AffinityOptionKeys() []string {
	return []string{
		OptionKeyRoutingAffinityEnabled,
		OptionKeyRoutingAffinityTTL,
		OptionKeyRoutingAffinityMode,
		OptionKeyRoutingAffinityCooldown,
		OptionKeyRoutingAffinityPrefixCache,
	}
}

// IsAffinityOptionKey reports whether the key is a routing-affinity option.
func IsAffinityOptionKey(key string) bool {
	switch key {
	case OptionKeyRoutingAffinityEnabled, OptionKeyRoutingAffinityTTL,
		OptionKeyRoutingAffinityMode, OptionKeyRoutingAffinityCooldown,
		OptionKeyRoutingAffinityPrefixCache:
		return true
	}
	return false
}

// RoutingAffinityEnabled reports whether sticky channel routing is turned on.
func RoutingAffinityEnabled() bool {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityEnabled
}

// RoutingAffinityTTL returns how long a remembered channel stays preferred.
func RoutingAffinityTTL() time.Duration {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityTTL
}

// RoutingAffinityMode returns the current affinity mode (sticky|hash).
func RoutingAffinityMode() string {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityMode
}

// RoutingAffinityCooldown returns how long a failed channel stays suppressed.
func RoutingAffinityCooldown() time.Duration {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityCooldown
}

// RoutingAffinityPrefixCacheEnabled reports whether prompt-prefix cache
// affinity is turned on (makes the key scope include the prefix hash).
func RoutingAffinityPrefixCacheEnabled() bool {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityPrefixCache
}

// UpdateAffinity applies a routing-affinity option key.
func UpdateAffinity(key, value string) error {
	switch key {
	case OptionKeyRoutingAffinityEnabled:
		v, err := parseBoolOption(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		routeAffMu.Lock()
		routingAffinityEnabled = v
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityTTL:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v <= 0 {
			return fmt.Errorf("invalid value for %s: must be > 0", key)
		}
		ttl := time.Duration(v) * time.Second
		if ttl > MaxRoutingAffinityTTL {
			ttl = MaxRoutingAffinityTTL
		}
		routeAffMu.Lock()
		routingAffinityTTL = ttl
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityMode:
		v := strings.TrimSpace(value)
		if v != AffinityModeSticky && v != AffinityModeHash {
			return fmt.Errorf("invalid value for %s: must be %q or %q", key, AffinityModeSticky, AffinityModeHash)
		}
		routeAffMu.Lock()
		routingAffinityMode = v
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityCooldown:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v <= 0 {
			return fmt.Errorf("invalid value for %s: must be > 0", key)
		}
		ttl := time.Duration(v) * time.Second
		if ttl > MaxRoutingAffinityCooldown {
			ttl = MaxRoutingAffinityCooldown
		}
		routeAffMu.Lock()
		routingAffinityCooldown = ttl
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityPrefixCache:
		v, err := parseBoolOption(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		routeAffMu.Lock()
		routingAffinityPrefixCache = v
		routeAffMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown affinity option key: %s", key)
	}
}

// SnapshotAffinity returns the persisted representation of a key.
func SnapshotAffinity(key string) (string, error) {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	switch key {
	case OptionKeyRoutingAffinityEnabled:
		return strconv.FormatBool(routingAffinityEnabled), nil
	case OptionKeyRoutingAffinityTTL:
		return strconv.FormatInt(int64(routingAffinityTTL/time.Second), 10), nil
	case OptionKeyRoutingAffinityMode:
		return routingAffinityMode, nil
	case OptionKeyRoutingAffinityCooldown:
		return strconv.FormatInt(int64(routingAffinityCooldown/time.Second), 10), nil
	case OptionKeyRoutingAffinityPrefixCache:
		return strconv.FormatBool(routingAffinityPrefixCache), nil
	default:
		return "", fmt.Errorf("unknown affinity option key: %s", key)
	}
}

// ResetAffinityToDefaults restores the compiled-in affinity defaults.
func ResetAffinityToDefaults() {
	routeAffMu.Lock()
	defer routeAffMu.Unlock()
	routingAffinityEnabled = false
	routingAffinityTTL = DefaultRoutingAffinityTTL
	routingAffinityMode = AffinityModeSticky
	routingAffinityCooldown = DefaultRoutingAffinityCooldown
	routingAffinityPrefixCache = false
}

// parseBoolOption accepts a JSON boolean as well as the common textual
// spellings an operator might type into the admin UI (quoted or not).
func parseBoolOption(value string) (bool, error) {
	trimmed := strings.TrimSpace(value)
	var raw any
	if err := json.Unmarshal([]byte(trimmed), &raw); err == nil {
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			trimmed = v
		}
	}
	switch strings.ToLower(trimmed) {
	case "1", "true", "on", "yes":
		return true, nil
	case "0", "false", "off", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", value)
}
