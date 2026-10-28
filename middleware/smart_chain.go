package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var errSmartChainHistory = errors.New("smart tool routing requires a known unexpired conversation; start from the first user message and preserve complete history or X-MyAPIs-Chain-Id")

// inferToolChain gives ordinary clients a stable pin without requiring a custom
// header. It scopes the first user content by endpoint; only its hash is stored.
// Identical prefixes within one token/group intentionally share a model pin.
func inferToolChain(path string, firstUser json.RawMessage) (string, error) {
	if len(firstUser) == 0 || bytes.Equal(bytes.TrimSpace(firstUser), []byte("null")) {
		return "", errSmartChainHistory
	}
	decoder := json.NewDecoder(bytes.NewReader(firstUser))
	decoder.UseNumber()
	var content any
	if err := decoder.Decode(&content); err != nil {
		return "", errSmartChainHistory
	}
	encoded, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content any    `json:"content"`
	}{path, content})
	if err != nil {
		return "", errSmartChainHistory
	}
	hash := sha256.Sum256(encoded)
	return "auto:" + hex.EncodeToString(hash[:]), nil
}
