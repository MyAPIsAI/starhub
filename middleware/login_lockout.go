package middleware

import (
	"sync"
	"time"
)

// LoginLockout tracks consecutive login failures per account (username or
// phone). After maxFailedAttempts failures within the tracking window, the
// account is locked for the lockDuration. This is an in-memory store
// consistent with the session store; multi-instance consistency requires T58.
type LoginLockout struct {
	mu       sync.Mutex
	attempts map[string]*attemptState
}

type attemptState struct {
	failures    int
	firstFail   time.Time
	lastFail    time.Time
	lockedUntil time.Time
}

const (
	maxFailedAttempts = 5
	lockDuration      = 15 * time.Minute
	attemptWindow     = 15 * time.Minute
)

var loginLockout = &LoginLockout{attempts: map[string]*attemptState{}}

// IsLocked reports whether the given account key is currently locked and
// returns the remaining lock duration.
func IsLocked(accountKey string) (bool, time.Duration) {
	loginLockout.mu.Lock()
	defer loginLockout.mu.Unlock()
	state, ok := loginLockout.attempts[accountKey]
	if !ok {
		return false, 0
	}
	if time.Now().Before(state.lockedUntil) {
		return true, time.Until(state.lockedUntil)
	}
	return false, 0
}

// RecordLoginFailure increments the failure counter for the account key.
// If the counter reaches maxFailedAttempts, the account is locked for
// lockDuration. Failures older than attemptWindow are expired.
func RecordLoginFailure(accountKey string) {
	loginLockout.mu.Lock()
	defer loginLockout.mu.Unlock()
	now := time.Now()
	state, ok := loginLockout.attempts[accountKey]
	if !ok || now.Sub(state.firstFail) > attemptWindow {
		state = &attemptState{firstFail: now}
		loginLockout.attempts[accountKey] = state
	}
	state.failures++
	state.lastFail = now
	if state.failures >= maxFailedAttempts {
		state.lockedUntil = now.Add(lockDuration)
	}
}

// RecordLoginSuccess clears the failure counter for the account key.
func RecordLoginSuccess(accountKey string) {
	loginLockout.mu.Lock()
	defer loginLockout.mu.Unlock()
	delete(loginLockout.attempts, accountKey)
}

// ResetLoginLockout clears all lockout state (used by tests).
func ResetLoginLockout() {
	loginLockout.mu.Lock()
	defer loginLockout.mu.Unlock()
	loginLockout.attempts = map[string]*attemptState{}
}
