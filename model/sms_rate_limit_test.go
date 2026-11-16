package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSMSRetryUsesLatestExhaustedWindowAndRollsBackReservations(t *testing.T) {
	newTestDB(t)
	minute := SMSRequestLimit{Scope: "fixture:minute", Window: time.Minute, Max: 1}
	hour := SMSRequestLimit{Scope: "fixture:hour", Window: time.Hour, Max: 1}
	free := SMSRequestLimit{Scope: "fixture:free", Window: time.Hour, Max: 1}
	if err := ReserveSMSRequest(minute, hour); err != nil {
		t.Fatal(err)
	}
	var limited *SMSRequestLimitError
	err := ReserveSMSRequest(free, minute, hour)
	if !errors.As(err, &limited) || !errors.Is(err, ErrSMSRequestLimited) {
		t.Fatal("missing typed retry information")
	}
	if seconds := limited.RetryAfterSeconds(); seconds < 3595 || seconds > 3600 {
		t.Fatalf("hourly limit reported %d seconds", seconds)
	}
	if err := ReserveSMSRequest(free); err != nil {
		t.Fatal("rejection consumed an unrelated budget")
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(hour.Scope)))
	if err := DB.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).UpdateColumn("started_at", time.Now().Add(-2*time.Hour).Unix()).Error; err != nil {
		t.Fatal(err)
	}
	err = ReserveSMSRequest(minute, hour)
	if !errors.As(err, &limited) {
		t.Fatal(err)
	}
	if seconds := limited.RetryAfterSeconds(); seconds < 55 || seconds > 60 {
		t.Fatalf("minute limit reported %d seconds", seconds)
	}
	// The expired hourly window is reset only when the complete reservation succeeds.
	key = fmt.Sprintf("%x", sha256.Sum256([]byte(minute.Scope)))
	if err := DB.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).UpdateColumn("started_at", time.Now().Add(-2*time.Minute).Unix()).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveSMSRequest(minute, hour); err != nil {
		t.Fatal(err)
	}
}
