package model

import (
	"sync"
	"time"
)

// AffinityKey is the scope of a sticky channel preference: one token asking
// for one model in one group. Group is part of the key so neither a model
// switch nor the auto-group walk (which changes group per hop) can drag a
// preference across scope boundaries.
type AffinityKey struct {
	TokenID int64
	Model   string
	Group   string
}

// affinityEntry is one remembered channel plus its expiry. The TTL keeps a
// stale preference from pinning traffic to a channel once traffic or upstream
// health shifts.
type affinityEntry struct {
	channelID int64
	expiresAt time.Time
}

// affinityStore is process-local sticky routing state; it resets on restart
// (and per test via ResetAffinity). It follows the multiKeyCounters pattern:
// a plain sync.Map with no external dependency. Deployments with more than one
// replica do NOT share it, so the same key may land on a different channel per
// replica; switching to a shared store or a stateless hash is a later phase.
var affinityStore sync.Map // AffinityKey -> affinityEntry

// RememberAffinity records channelID as the preferred channel for key until
// ttl elapses. A non-positive channelID or ttl clears the entry instead.
func RememberAffinity(key AffinityKey, channelID int64, ttl time.Duration) {
	if channelID <= 0 || ttl <= 0 {
		affinityStore.Delete(key)
		return
	}
	affinityStore.Store(key, affinityEntry{
		channelID: channelID,
		expiresAt: time.Now().Add(ttl),
	})
}

// AffinityChannel returns the remembered channel for key when it has not
// expired. An expired entry is removed on the way out.
func AffinityChannel(key AffinityKey) (int64, bool) {
	raw, ok := affinityStore.Load(key)
	if !ok {
		return 0, false
	}
	entry := raw.(affinityEntry)
	if time.Now().After(entry.expiresAt) {
		affinityStore.CompareAndDelete(key, entry)
		return 0, false
	}
	return entry.channelID, true
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

// ResetAffinity clears every sticky preference (used by tests).
func ResetAffinity() {
	affinityStore = sync.Map{}
}
