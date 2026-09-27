package model

import (
	"sync"
	"time"
)

// AffinityKey is the scope of a sticky channel preference: one token asking
// for one model in one group. Group is part of the key so neither a model
// switch nor the auto-group walk (which changes group per hop) can drag a
// preference across scope boundaries. Prefix adds an optional prompt-prefix
// scope so prefix-cache affinity can keep a conversation on the upstream that
// already holds its cache; it is empty when prefix affinity is disabled.
type AffinityKey struct {
	TokenID int64
	Model   string
	Group   string
	Prefix  string
}

// affinityEntry is one remembered channel (and optional upstream key index)
// plus its expiry. The TTL keeps a stale preference from pinning traffic to a
// channel once traffic or upstream health shifts.
type affinityEntry struct {
	channelID int64
	keyIdx    int // upstream key index for multi-key channels; -1 when unknown
	expiresAt time.Time
}

// affinityStore is process-local sticky routing state; it resets on restart
// (and per test via ResetAffinity). It follows the multiKeyCounters pattern: a
// plain sync.Map with no external dependency. Deployments with more than one
// replica do NOT share it, so the same key may land on a different channel per
// replica; the hash affinity mode (stateless rendezvous) is the replica-safe
// alternative.
var affinityStore sync.Map // AffinityKey -> affinityEntry

// affinityCooldown is process-local per-channel suppression: after a retryable
// failure the channel is not used as a sticky preference for a short window,
// so the next request does not immediately hit the same broken upstream again.
// It is keyed by channel ID and shared across affinity keys.
var affinityCooldown sync.Map // int64 channelID -> time.Time expiresAt

// RememberAffinity records channelID (and the upstream key index, -1 when
// unknown or not applicable) as the preferred channel for key until ttl
// elapses. A non-positive channelID or ttl clears the entry instead.
func RememberAffinity(key AffinityKey, channelID int64, keyIdx int, ttl time.Duration) {
	if channelID <= 0 || ttl <= 0 {
		affinityStore.Delete(key)
		return
	}
	affinityStore.Store(key, affinityEntry{
		channelID: channelID,
		keyIdx:    keyIdx,
		expiresAt: time.Now().Add(ttl),
	})
}

// AffinityChannel returns the remembered channel for key when it has not
// expired. An expired entry is removed on the way out.
func AffinityChannel(key AffinityKey) (int64, bool) {
	entry, ok := loadAffinity(key)
	if !ok {
		return 0, false
	}
	return entry.channelID, true
}

// AffinityKeyIndex returns the remembered upstream key index for key. The
// second result is false when no entry exists or no key index was recorded
// (single-key channels store -1).
func AffinityKeyIndex(key AffinityKey) (int, bool) {
	entry, ok := loadAffinity(key)
	if !ok || entry.keyIdx < 0 {
		return -1, false
	}
	return entry.keyIdx, true
}

func loadAffinity(key AffinityKey) (affinityEntry, bool) {
	raw, ok := affinityStore.Load(key)
	if !ok {
		return affinityEntry{}, false
	}
	entry := raw.(affinityEntry)
	if time.Now().After(entry.expiresAt) {
		affinityStore.CompareAndDelete(key, entry)
		return affinityEntry{}, false
	}
	return entry, true
}

// ForgetAffinityIf clears the preference for key only when it still points at
// channelID. The compare avoids a race where a failed attempt on an old
// channel deletes a fresh preference written by a concurrent successful
// request.
func ForgetAffinityIf(key AffinityKey, channelID int64) {
	raw, ok := affinityStore.Load(key)
	if !ok {
		return
	}
	entry := raw.(affinityEntry)
	if entry.channelID == channelID {
		affinityStore.CompareAndDelete(key, entry)
	}
}

// CoolDownChannel suppresses channelID as a sticky preference until ttl
// elapses. A non-positive ttl clears the suppression.
func CoolDownChannel(channelID int64, ttl time.Duration) {
	if channelID <= 0 || ttl <= 0 {
		affinityCooldown.Delete(channelID)
		return
	}
	affinityCooldown.Store(channelID, time.Now().Add(ttl))
}

// ChannelCoolingDown reports whether channelID is still suppressed. An expired
// entry is pruned on read.
func ChannelCoolingDown(channelID int64) bool {
	raw, ok := affinityCooldown.Load(channelID)
	if !ok {
		return false
	}
	expiresAt := raw.(time.Time)
	if time.Now().After(expiresAt) {
		affinityCooldown.CompareAndDelete(channelID, expiresAt)
		return false
	}
	return true
}

// ResetAffinity clears every sticky preference and cooldown (used by tests).
func ResetAffinity() {
	affinityStore = sync.Map{}
	affinityCooldown = sync.Map{}
}
