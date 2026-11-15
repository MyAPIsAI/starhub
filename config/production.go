package config

import (
	"errors"
	"strings"
)

// ValidateProduction rejects disposable secrets and bootstrap credentials when
// the deployment explicitly selects production mode.
func ValidateProduction(bootstrap Bootstrap, getenv func(string) string) error {
	mode := strings.TrimSpace(getenv("STARHUB_ENV"))
	if mode != "" && mode != "development" && mode != "production" {
		return errors.New("STARHUB_ENV must be development or production")
	}
	if mode != "production" {
		return nil
	}
	if len(getenv("STARHUB_CRYPTO_SECRET")) < 32 {
		return errors.New("production requires a persistent STARHUB_CRYPTO_SECRET of at least 32 characters")
	}
	if len(bootstrap.RootPassword) < 12 {
		return errors.New("production requires STARHUB_ROOT_PASSWORD of at least 12 characters")
	}
	return nil
}
