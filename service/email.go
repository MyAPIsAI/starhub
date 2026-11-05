package service

import (
	"errors"
	"sync"
)

// EmailProvider is the interface for sending email verification codes and
// security notifications. Implementations must not log the code.
type EmailProvider interface {
	SendVerification(toEmail, code string) error
	SendNotification(toEmail, subject, body string) error
}

var (
	emailMu       sync.RWMutex
	emailProvider EmailProvider = &disabledEmailProvider{}
	emailEnabled  bool
)

type disabledEmailProvider struct{}

func (d *disabledEmailProvider) SendVerification(toEmail, code string) error {
	return ErrEmailDisabled
}

func (d *disabledEmailProvider) SendNotification(toEmail, subject, body string) error {
	return ErrEmailDisabled
}

// ErrEmailDisabled is returned when no email provider is configured.
var ErrEmailDisabled = errors.New("email service is not enabled")

// SetEmailProvider replaces the active email provider and enables email.
func SetEmailProvider(provider EmailProvider) {
	emailMu.Lock()
	defer emailMu.Unlock()
	emailProvider = provider
	if provider != nil {
		emailEnabled = true
	} else {
		emailProvider = &disabledEmailProvider{}
		emailEnabled = false
	}
}

// IsEmailEnabled reports whether a real email provider is configured.
func IsEmailEnabled() bool {
	emailMu.RLock()
	defer emailMu.RUnlock()
	return emailEnabled
}

// SendEmailVerification dispatches a verification code email.
func SendEmailVerification(toEmail, code string) error {
	emailMu.RLock()
	provider := emailProvider
	emailMu.RUnlock()
	return provider.SendVerification(toEmail, code)
}

// SendEmailNotification dispatches a security notification email.
func SendEmailNotification(toEmail, subject, body string) error {
	emailMu.RLock()
	provider := emailProvider
	emailMu.RUnlock()
	return provider.SendNotification(toEmail, subject, body)
}
