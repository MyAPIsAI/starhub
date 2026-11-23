package service

import (
	"sync/atomic"

	"github.com/google/uuid"
)

// Fixed process-lifetime counters: no request-controlled labels or session values.
type affinityEvent int

type AffinityBypassReason string

const (
	AffinityBypassDisabled     AffinityBypassReason = "disabled"
	AffinityBypassMissingToken AffinityBypassReason = "missing_token"
	AffinityBypassHashMode     AffinityBypassReason = "hash_mode"
	AffinityBypassTraffic      AffinityBypassReason = "traffic_allocation"
)

const (
	conditionsUnmatched affinityEvent = iota
	affinityDisabled
	missingToken
	hashModeBypass
	missingSource
	invalidValue
	ruleOff
	coldStart
	ruleHit
	legacyHit
	writeOK
	writeRevision
	writeClear
	writeRaced
	writeFull
	trafficAllocation
	eventCount
)

var eventNames = [eventCount]string{"conditions_unmatched", "affinity_disabled", "missing_token", "hash_mode_bypass", "missing_source", "invalid_value", "rule_off", "cold_start", "rule_hit", "legacy_hit", "write_ok", "write_revision", "write_clear", "write_raced", "write_full", "traffic_allocation"}
var affinityEvents [eventCount]atomic.Uint64
var affinityInstance = uuid.NewString()

func (e affinityEvent) count() { affinityEvents[e].Add(1) }

func affinityEventSnapshot() map[string]uint64 {
	out := make(map[string]uint64, eventCount)
	for i, name := range eventNames {
		out[name] = affinityEvents[i].Load()
	}
	return out
}

func AffinityInstance() string { return affinityInstance }

// RecordAffinitySelection counts actual preferred attempts, not admission picks.
func RecordAffinitySelection(rule bool) {
	if rule {
		ruleHit.count()
	} else {
		legacyHit.count()
	}
}

// RecordAffinityBypass records why configured session rules were not evaluated.
// Reasons are fixed values so request data never becomes a metric label.
func RecordAffinityBypass(reason AffinityBypassReason) {
	switch reason {
	case AffinityBypassTraffic:
		trafficAllocation.count()
	case AffinityBypassDisabled:
		affinityDisabled.count()
	case AffinityBypassMissingToken:
		missingToken.count()
	case AffinityBypassHashMode:
		hashModeBypass.count()
	}
}
