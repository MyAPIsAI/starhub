package common

import (
	"crypto/rand"
	"fmt"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret creates a new TOTP secret for the given account name.
// Returns the secret string and the OTP Key (which contains the URL for
// QR code generation).
func GenerateTOTPSecret(accountName string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      "StarHub",
		AccountName: accountName,
		Period:      30,
		SecretSize:  20,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
}

// ValidateTOTPCode validates a 6-digit TOTP code against the given secret.
// Uses a ±1 period skew to allow for clock drift.
func ValidateTOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes generates 4 random 8-character backup codes in
// XXXX-XXXX format using crypto/rand.
func GenerateBackupCodes() ([]string, error) {
	const charset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	codes := make([]string, 4)
	for i := range codes {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		chars := make([]byte, 8)
		for j := range chars {
			chars[j] = charset[int(raw[j])%len(charset)]
		}
		codes[i] = fmt.Sprintf("%s-%s", string(chars[:4]), string(chars[4:]))
	}
	return codes, nil
}
