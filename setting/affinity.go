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
//	routing_affinity_enabled      false  remember the channel that last served
//	    a (token, model, group) and prefer it on the next request. Disabled by
//	    default: the weighted draw is unchanged unless an operator opts in.
//	routing_affinity_ttl_seconds  300    how long a remembered channel stays
//	    sticky before the weighted draw takes over again.
const (
	OptionKeyRoutingAffinityEnabled = "routing_affinity_enabled"
	OptionKeyRoutingAffinityTTL     = "routing_affinity_ttl_seconds"
)

const (
	// DefaultRoutingAffinityTTL is the sticky window used when the option is
	// unset. Short enough that a stale preference cannot pin a channel for
	// long once traffic or upstream health shifts.
	DefaultRoutingAffinityTTL = 5 * time.Minute
	// MaxRoutingAffinityTTL caps the sticky window so a misconfiguration
	// cannot defeat load spreading for days.
	MaxRoutingAffinityTTL = 24 * time.Hour
	// MaxRoutingAffinityTTLSeconds is the numeric form for the normalize
	// function, which does not import the time package.
	MaxRoutingAffinityTTLSeconds = int64(24 * 60 * 60)
)

var (
	routeAffMu             sync.RWMutex
	routingAffinityEnabled = false
	routingAffinityTTL     = DefaultRoutingAffinityTTL
)

// AffinityOptionKeys lists the option keys owned by this file.
func AffinityOptionKeys() []string {
	return []string{OptionKeyRoutingAffinityEnabled, OptionKeyRoutingAffinityTTL}
}

// IsAffinityOptionKey reports whether the key is a routing-affinity option.
func IsAffinityOptionKey(key string) bool {
	switch key {
	case OptionKeyRoutingAffinityEnabled, OptionKeyRoutingAffinityTTL:
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
	default:
		return "", fmt.Errorf("unknown affinity option key: %s", key)
	}
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

// ResetAffinityToDefaults restores the compiled-in affinity defaults.
func ResetAffinityToDefaults() {
	routeAffMu.Lock()
	defer routeAffMu.Unlock()
	routingAffinityEnabled = false
	routingAffinityTTL = DefaultRoutingAffinityTTL
}
