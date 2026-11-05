package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

const (
	tokenKeyChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	tokenKeyLength = 48
	tokenKeyPrefix = "sk-"
)

// cryptoSecret is a process-wide random key used for HMAC hashing of
// verification codes. It is generated once at startup and never persisted.
var (
	cryptoSecretOnce sync.Once
	cryptoSecret     []byte
)

func getCryptoSecret() []byte {
	cryptoSecretOnce.Do(func() {
		cryptoSecret = make([]byte, 32)
		_, _ = rand.Read(cryptoSecret)
	})
	return cryptoSecret
}

// GenerateHMAC returns a hex-encoded HMAC-SHA256 of the given message using
// the process-wide crypto secret. Used for hashing verification codes so
// they are never stored in plaintext.
func GenerateHMAC(message string) string {
	mac := hmac.New(sha256.New, getCryptoSecret())
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func GenerateTokenKey() string {
	raw := make([]byte, tokenKeyLength)
	_, _ = rand.Read(raw)
	for i := range raw {
		raw[i] = tokenKeyChars[int(raw[i])%len(tokenKeyChars)]
	}
	return tokenKeyPrefix + string(raw)
}

func GenerateSessionKey() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func MaskKey(s string) string {
	if len(s) <= 12 {
		return "******"
	}
	return s[:6] + "******" + s[len(s)-4:]
}

// GenerateAffCode returns a random 8-character invite code. The alphabet
// avoids ambiguous characters (0/O, 1/I/l) so the code is readable when
// shared verbally.
func GenerateAffCode() string {
	const affChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	for i := range raw {
		raw[i] = affChars[int(raw[i])%len(affChars)]
	}
	return string(raw)
}
