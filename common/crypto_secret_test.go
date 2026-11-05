package common

import (
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plaintexts := []string{
		"sk-abc123def456",
		"sk-test-key-with-special-chars!@#",
		"short",
		"a",
		"",
	}
	for _, pt := range plaintexts {
		encrypted, err := Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q) failed: %v", pt, err)
		}
		if pt == "" {
			if encrypted != "" {
				t.Fatalf("empty plaintext should produce empty ciphertext, got %q", encrypted)
			}
			continue
		}
		if !IsEncrypted(encrypted) {
			t.Fatalf("ciphertext should have enc: prefix: %q", encrypted)
		}
		decrypted, err := Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}
		if decrypted != pt {
			t.Fatalf("roundtrip mismatch: got %q, want %q", decrypted, pt)
		}
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	// Values without the enc: prefix should pass through unchanged.
	cases := []string{"sk-plaintext", "anything", ""}
	for _, pt := range cases {
		result, err := Decrypt(pt)
		if err != nil {
			t.Fatalf("Decrypt(%q) failed: %v", pt, err)
		}
		if result != pt {
			t.Fatalf("passthrough mismatch: got %q, want %q", result, pt)
		}
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	pt := "sk-same-input"
	c1, _ := Encrypt(pt)
	c2, _ := Encrypt(pt)
	if c1 == c2 {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts (random nonce)")
	}
	// Both should decrypt to the same value.
	d1, _ := Decrypt(c1)
	d2, _ := Decrypt(c2)
	if d1 != pt || d2 != pt {
		t.Fatal("both ciphertexts should decrypt to the original plaintext")
	}
}

func TestHashKeyDeterministic(t *testing.T) {
	key := "sk-test-key-123"
	h1 := HashKey(key)
	h2 := HashKey(key)
	if h1 != h2 {
		t.Fatal("HashKey should be deterministic for the same input within one process")
	}
	if h1 == "" {
		t.Fatal("HashKey should not return empty for non-empty input")
	}
}

func TestHashKeyDifferentInputs(t *testing.T) {
	h1 := HashKey("sk-key-one")
	h2 := HashKey("sk-key-two")
	if h1 == h2 {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestHashKeyEmpty(t *testing.T) {
	if h := HashKey(""); h != "" {
		t.Fatalf("HashKey(\"\") = %q, want empty", h)
	}
}

func TestIsEncrypted(t *testing.T) {
	if IsEncrypted("sk-plaintext") {
		t.Fatal("plaintext should not be detected as encrypted")
	}
	enc, _ := Encrypt("test")
	if !IsEncrypted(enc) {
		t.Fatal("encrypted value should be detected as encrypted")
	}
	if IsEncrypted("") {
		t.Fatal("empty string should not be detected as encrypted")
	}
}
