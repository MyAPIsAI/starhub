package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
)

const CredentialEnvelopePrefix = "enc:v2:"

func credentialKeys() (map[string][]byte, string, error) {
	raw, active := os.Getenv("STARHUB_CREDENTIAL_KEYS"), os.Getenv("STARHUB_CREDENTIAL_ACTIVE_KEY")
	if raw == "" && active == "" {
		return map[string][]byte{"legacy": getEncryptionKey()}, "legacy", nil
	}
	var values map[string]string
	if json.Unmarshal([]byte(raw), &values) != nil || len(values) == 0 || len(values) > 8 {
		return nil, "", errors.New("invalid credential key configuration")
	}
	keys := make(map[string][]byte, len(values))
	for id, value := range values {
		key, err := hex.DecodeString(value)
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`).MatchString(id) || err != nil || len(key) != 32 {
			return nil, "", errors.New("invalid credential key configuration")
		}
		keys[id] = key
	}
	if keys[active] == nil {
		return nil, "", errors.New("active credential key is unavailable")
	}
	return keys, active, nil
}

func credentialSeal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrEncryptionFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrEncryptionFailed
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrEncryptionFailed
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func credentialOpen(key, stored, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrEncryptionFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(stored) < gcm.NonceSize() {
		return nil, ErrEncryptionFailed
	}
	value, err := gcm.Open(nil, stored[:gcm.NonceSize()], stored[gcm.NonceSize():], aad)
	if err != nil {
		return nil, ErrEncryptionFailed
	}
	return value, nil
}

// EncryptCredential uses a fresh data key per value and wraps it with the
// active versioned key. Purpose is authenticated to prevent cross-field reuse.
func EncryptCredential(plaintext, purpose string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if purpose != "customer" && purpose != "upstream" {
		return "", ErrEncryptionFailed
	}
	keys, active, err := credentialKeys()
	if err != nil {
		return "", err
	}
	dataKey := make([]byte, 32)
	if _, err := rand.Read(dataKey); err != nil {
		return "", ErrEncryptionFailed
	}
	aad := []byte("starhub-credential-v2|" + purpose + "|" + active)
	wrapped, err := credentialSeal(keys[active], dataKey, aad)
	if err != nil {
		return "", err
	}
	payload, err := credentialSeal(dataKey, []byte(plaintext), aad)
	if err != nil {
		return "", err
	}
	return CredentialEnvelopePrefix + active + ":" + base64.RawURLEncoding.EncodeToString(wrapped) + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecryptCredential(stored, purpose string) (string, error) {
	if !strings.HasPrefix(stored, CredentialEnvelopePrefix) {
		return Decrypt(stored)
	}
	parts := strings.Split(strings.TrimPrefix(stored, CredentialEnvelopePrefix), ":")
	if len(parts) != 3 || (purpose != "customer" && purpose != "upstream") {
		return "", ErrEncryptionFailed
	}
	keys, _, err := credentialKeys()
	if err != nil {
		return "", err
	}
	if keys[parts[0]] == nil {
		return "", ErrEncryptionFailed
	}
	wrapped, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrEncryptionFailed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrEncryptionFailed
	}
	aad := []byte("starhub-credential-v2|" + purpose + "|" + parts[0])
	dataKey, err := credentialOpen(keys[parts[0]], wrapped, aad)
	if err != nil || len(dataKey) != 32 {
		return "", ErrEncryptionFailed
	}
	plaintext, err := credentialOpen(dataKey, payload, aad)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
