package common

import "testing"

func TestHashPasswordRoundtrip(t *testing.T) {
	hashed, err := HashPassword("secret-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !VerifyPassword("secret-password", hashed) {
		t.Fatal("VerifyPassword should accept the correct password")
	}
	if VerifyPassword("wrong-password", hashed) {
		t.Fatal("VerifyPassword should reject a wrong password")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	if VerifyPassword("x", "") {
		t.Fatal("empty stored hash should not verify")
	}
	if VerifyPassword("x", "not-a-valid-hash") {
		t.Fatal("garbage stored hash should not verify")
	}
	if VerifyPassword("x", "pbkdf2-sha256$abc$AAAA$BBBB") {
		t.Fatal("malformed iteration count should not verify")
	}
}

func TestMaskKey(t *testing.T) {
	short := "sk-mini-key"
	if got := MaskKey(short); got != "******" {
		t.Fatalf("short key should be fully masked, got %q", got)
	}
	long := "sk-abcdefghijklmnop"
	got := MaskKey(long)
	if got == long || len(got) < 10 {
		t.Fatalf("long key should be partially masked, got %q", got)
	}
	if got[:6] != long[:6] || got[len(got)-4:] != long[len(long)-4:] {
		t.Fatalf("mask should keep prefix and suffix, got %q", got)
	}
}

func TestGenerateTokenKey(t *testing.T) {
	key := GenerateTokenKey()
	if len(key) != 3+48 {
		t.Fatalf("unexpected key length: %d", len(key))
	}
	if key[:3] != "sk-" {
		t.Fatalf("key should carry the sk- prefix: %q", key)
	}
	if key == GenerateTokenKey() {
		t.Fatal("generated keys should be unique")
	}
}
