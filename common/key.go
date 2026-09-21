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
