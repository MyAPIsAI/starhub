package common

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestCredentialEnvelopeRetainsLegacyCustomerReadCompatibility(t *testing.T) {
	legacy, err := credentialSeal(getEncryptionKey(), []byte("legacy-customer-key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	value, err := Decrypt("enc:" + hex.EncodeToString(legacy))
	if err != nil || value != "legacy-customer-key" {
		t.Fatal("legacy customer key became unreadable")
	}
	upstream, err := EncryptCredential("upstream-key", "upstream")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(upstream); err == nil {
		t.Fatal("customer decrypt accepted upstream credential")
	}
}

func TestCredentialKeyringRejectsMalformedConfigurations(t *testing.T) {
	key := strings.Repeat("11", 32)
	large := map[string]string{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		large[id] = key
	}
	oversized, _ := json.Marshal(large)
	for _, tc := range []struct{ name, raw, active string }{
		{"invalid-json", "{", "a"}, {"empty-object", "{}", "a"}, {"null", "null", "a"}, {"wrong-type", "[]", "a"},
		{"no-active", `{"a":"` + key + `"}`, ""}, {"missing-active", `{"a":"` + key + `"}`, "b"},
		{"invalid-id", `{"a:b":"` + key + `"}`, "a:b"}, {"short-key", `{"a":"11"}`, "a"},
		{"non-hex", `{"a":"` + strings.Repeat("zz", 32) + `"}`, "a"}, {"too-many", string(oversized), "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STARHUB_CREDENTIAL_KEYS", tc.raw)
			t.Setenv("STARHUB_CREDENTIAL_ACTIVE_KEY", tc.active)
			if ciphertext, err := EncryptCredential("test-value", "upstream"); err == nil || ciphertext != "" {
				t.Fatal("invalid keyring fell back to a usable key")
			}
		})
	}
}

func TestCredentialEnvelopePurposeTamperingAndRotation(t *testing.T) {
	key1, key2 := strings.Repeat("11", 32), strings.Repeat("22", 32)
	setKeys := func(keys map[string]string, active string) {
		t.Helper()
		value, err := json.Marshal(keys)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("STARHUB_CREDENTIAL_KEYS", string(value))
		t.Setenv("STARHUB_CREDENTIAL_ACTIVE_KEY", active)
	}
	setKeys(map[string]string{"first": key1}, "first")
	beforeHash := HashKey("customer-test-key")
	old, err := EncryptCredential("upstream-test-key", "upstream")
	if err != nil {
		t.Fatal(err)
	}
	other, err := EncryptCredential("upstream-test-key", "upstream")
	if err != nil || old == other {
		t.Fatal("envelopes must be randomized")
	}
	if plaintext, err := DecryptCredential(old, "upstream"); err != nil || plaintext != "upstream-test-key" {
		t.Fatal("round trip failed")
	}
	if _, err := DecryptCredential(old, "customer"); err == nil {
		t.Fatal("cross-purpose credential accepted")
	}
	if _, err := DecryptCredential(old[:len(old)-2]+"xx", "upstream"); err == nil {
		t.Fatal("tampered credential accepted")
	}
	setKeys(map[string]string{"first": key1, "second": key2}, "second")
	plaintext, err := DecryptCredential(old, "upstream")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := EncryptCredential(plaintext, "upstream")
	if err != nil || !strings.HasPrefix(rotated, "enc:v2:second:") {
		t.Fatal("active key not used")
	}
	if HashKey("customer-test-key") != beforeHash {
		t.Fatal("envelope rotation changed customer lookup identity")
	}
	setKeys(map[string]string{"second": key2}, "second")
	if _, err := DecryptCredential(old, "upstream"); err == nil {
		t.Fatal("removed wrapping key still accepted")
	}
	if plaintext, err := DecryptCredential(rotated, "upstream"); err != nil || plaintext != "upstream-test-key" {
		t.Fatal("rotated credential unreadable")
	}
	t.Setenv("STARHUB_CREDENTIAL_KEYS", `{"broken":"invalid"}`)
	if _, err := EncryptCredential("secret", "upstream"); err == nil {
		t.Fatal("bad configuration silently fell back")
	}
}
