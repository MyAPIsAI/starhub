package common

import (
	"crypto/rand"
	"encoding/hex"
)

const (
	tokenKeyChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	tokenKeyLength = 48
	tokenKeyPrefix = "sk-"
)

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
