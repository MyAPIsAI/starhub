package middleware

import (
	"testing"
	"time"
)

func TestLoginLockoutNotLockedByDefault(t *testing.T) {
	ResetLoginLockout()
	locked, _ := IsLocked("test-user")
	if locked {
		t.Fatal("should not be locked by default")
	}
}

func TestLoginLockoutAfterFiveFailures(t *testing.T) {
	ResetLoginLockout()
	for i := 0; i < 5; i++ {
		RecordLoginFailure("test-user")
	}
	locked, remaining := IsLocked("test-user")
	if !locked {
		t.Fatal("should be locked after 5 failures")
	}
	if remaining <= 0 || remaining > 15*time.Minute {
		t.Fatalf("remaining = %v, expected between 0 and 15 minutes", remaining)
	}
}

func TestLoginLockoutFourFailuresNotLocked(t *testing.T) {
	ResetLoginLockout()
	for i := 0; i < 4; i++ {
		RecordLoginFailure("test-user")
	}
	locked, _ := IsLocked("test-user")
	if locked {
		t.Fatal("should not be locked after only 4 failures")
	}
}

func TestLoginLockoutSuccessClearsCounter(t *testing.T) {
	ResetLoginLockout()
	RecordLoginFailure("test-user")
	RecordLoginFailure("test-user")
	RecordLoginSuccess("test-user")
	locked, _ := IsLocked("test-user")
	if locked {
		t.Fatal("should not be locked after success clears counter")
	}
	// A single new failure should not lock (counter was reset).
	RecordLoginFailure("test-user")
	locked, _ = IsLocked("test-user")
	if locked {
		t.Fatal("should not be locked with only 1 failure after reset")
	}
}

func TestLoginLockoutDifferentAccounts(t *testing.T) {
	ResetLoginLockout()
	RecordLoginFailure("user-a")
	RecordLoginFailure("user-a")
	locked, _ := IsLocked("user-a")
	if locked {
		t.Fatal("user-a should not be locked with only 2 failures")
	}
	locked, _ = IsLocked("user-b")
	if locked {
		t.Fatal("user-b should not be locked")
	}
}

func TestLoginLockoutExpiresAfterDuration(t *testing.T) {
	ResetLoginLockout()
	for i := 0; i < 5; i++ {
		RecordLoginFailure("expiring-user")
	}
	locked, _ := IsLocked("expiring-user")
	if !locked {
		t.Fatal("should be locked")
	}
	// Simulate expiry by manipulating internal state.
	loginLockout.mu.Lock()
	state := loginLockout.attempts["expiring-user"]
	state.lockedUntil = time.Now().Add(-1 * time.Second)
	loginLockout.mu.Unlock()
	locked, _ = IsLocked("expiring-user")
	if locked {
		t.Fatal("should not be locked after lock expires")
	}
}
