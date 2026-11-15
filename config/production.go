package config

import (
	"errors"
	"strings"
	"unicode"
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
	if len(strings.TrimSpace(getenv("STARHUB_CRYPTO_SECRET"))) < 32 {
		return errors.New("production requires a persistent STARHUB_CRYPTO_SECRET of at least 32 characters")
	}
	letter, digit := false, false
	for _, r := range bootstrap.RootPassword {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	if len(bootstrap.RootPassword) < 12 || len(bootstrap.RootPassword) > 64 || strings.TrimSpace(bootstrap.RootPassword) != bootstrap.RootPassword || !letter || !digit {
		return errors.New("production requires a 12-64 byte STARHUB_ROOT_PASSWORD containing letters and digits")
	}
	return nil
}
