package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
)

// EncryptedPrefix marks a stored value as AES-256-GCM encrypted so the
// migration path can distinguish plaintext from ciphertext.
const EncryptedPrefix = "enc:"

var (
	encryptionKeyOnce sync.Once
	encryptionKey     []byte
)

// ErrEncryptionFailed is returned when encryption or decryption fails.
var ErrEncryptionFailed = errors.New("encryption operation failed")

// getEncryptionKey returns the process-wide AES-256 key used for
// credential encryption at rest. The key is sourced from the
// STARHUB_CRYPTO_SECRET environment variable (32 bytes hex-encoded or raw).
// When unset, a random key is generated — encrypted data will not survive
// a restart, but the system still functions for development.
func getEncryptionKey() []byte {
	encryptionKeyOnce.Do(func() {
		if secret := os.Getenv("STARHUB_CRYPTO_SECRET"); secret != "" {
			// Try hex decode first (64 hex chars = 32 bytes).
			if decoded, err := hex.DecodeString(secret); err == nil && len(decoded) == 32 {
				encryptionKey = decoded
				return
			}
			// Fall back to SHA-256 of the raw string (any length → 32 bytes).
			h := sha256.Sum256([]byte(secret))
			encryptionKey = h[:]
			return
		}
		// No env var: generate a random key (development mode).
		encryptionKey = make([]byte, 32)
		_, _ = rand.Read(encryptionKey)
	})
	return encryptionKey
}

// Encrypt writes versioned customer envelopes. Legacy values remain readable.
func Encrypt(plaintext string) (string, error) {
	return EncryptCredential(plaintext, "customer")
}

// Decrypt decrypts a value produced by Encrypt. If the value does not have
// the "enc:" prefix, it is returned as-is (backward compatible with
// plaintext stored before migration).
func Decrypt(stored string) (string, error) {
	if strings.HasPrefix(stored, CredentialEnvelopePrefix) {
		return DecryptCredential(stored, "customer")
	}
	if stored == "" {
		return "", nil
	}
	if len(stored) < len(EncryptedPrefix) || stored[:len(EncryptedPrefix)] != EncryptedPrefix {
		// Plaintext (not yet migrated) — return as-is.
		return stored, nil
	}
	raw, err := hex.DecodeString(stored[len(EncryptedPrefix):])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(getEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrEncryptionFailed
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// HashKey returns a hex-encoded SHA-256 hash of a key for use as a
// non-reversible lookup index. The hash is keyed with the encryption key
// (HMAC-SHA256) so it cannot be brute-forced without the process secret.
func HashKey(key string) string {
	if key == "" {
		return ""
	}
	mac := hmac.New(sha256.New, getEncryptionKey())
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

// IsEncrypted reports whether a stored value has the encryption prefix.
func IsEncrypted(stored string) bool {
	return len(stored) > len(EncryptedPrefix) && stored[:len(EncryptedPrefix)] == EncryptedPrefix
}
