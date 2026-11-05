package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

// SMSProvider is the interface for sending SMS verification codes.
// Implementations must not log the code or the full phone number.
type SMSProvider interface {
	Send(phoneE164, code string) error
}

// ErrSMSSendFailed is returned when the SMS provider fails to send.
var ErrSMSSendFailed = errors.New("sms send failed")
var ErrSMSRateLimited = errors.New("sms rate limited by provider")
var ErrSMSDisabled = errors.New("sms service is not enabled")

// smsProviderHolder holds the active SMS provider. It defaults to a disabled
// provider that returns ErrSMSDisabled. Admins configure a real provider
// via SetSMSProvider.
var (
	smsMu       sync.RWMutex
	smsProvider SMSProvider = &disabledSMSProvider{}
	smsEnabled  bool
)

type disabledSMSProvider struct{}

func (d *disabledSMSProvider) Send(phoneE164, code string) error {
	return ErrSMSDisabled
}

// SetSMSProvider replaces the active SMS provider and enables SMS sending.
func SetSMSProvider(provider SMSProvider) {
	smsMu.Lock()
	defer smsMu.Unlock()
	smsProvider = provider
	if provider != nil {
		smsEnabled = true
	} else {
		smsProvider = &disabledSMSProvider{}
		smsEnabled = false
	}
}

// IsSMSEnabled reports whether a real SMS provider is configured.
func IsSMSEnabled() bool {
	smsMu.RLock()
	defer smsMu.RUnlock()
	return smsEnabled
}

// SendSMSVerification dispatches to the active provider.
func SendSMSVerification(phoneE164, code string) error {
	smsMu.RLock()
	provider := smsProvider
	smsMu.RUnlock()
	return provider.Send(phoneE164, code)
}

// GenerateNumericVerificationCode produces a cryptographically secure 6-digit
// numeric code using crypto/rand.
func GenerateNumericVerificationCode() (string, error) {
	digits := make([]byte, 6)
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	for i := range digits {
		digits[i] = '0' + (buf[i] % 10)
	}
	return string(digits), nil
}
