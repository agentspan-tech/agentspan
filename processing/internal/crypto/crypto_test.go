package crypto_test

import (
	"strings"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/crypto"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const wrongKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := []byte("hello, encryption!")
	encrypted, err := crypto.Encrypt(plaintext, testKeyHex)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := crypto.Decrypt(encrypted, testKeyHex)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("roundtrip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptDifferentKey(t *testing.T) {
	plaintext := []byte("secret data")
	encrypted, err := crypto.Encrypt(plaintext, testKeyHex)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = crypto.Decrypt(encrypted, wrongKeyHex)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key, got nil")
	}
}

func TestEncryptInvalidKeyHex(t *testing.T) {
	// Non-hex string
	_, err := crypto.Encrypt([]byte("test"), "not-hex-at-all!")
	if err == nil {
		t.Error("expected error for non-hex key, got nil")
	}

	// Wrong length (too short)
	_, err = crypto.Encrypt([]byte("test"), "abcdef")
	if err == nil {
		t.Error("expected error for short key, got nil")
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	// Empty ciphertext
	_, err := crypto.Decrypt([]byte{}, testKeyHex)
	if err == nil {
		t.Error("expected error for empty ciphertext, got nil")
	}

	// Truncated ciphertext (just version byte + a few bytes)
	_, err = crypto.Decrypt([]byte{0x01, 0x02, 0x03}, testKeyHex)
	if err == nil {
		t.Error("expected error for truncated ciphertext, got nil")
	}

	// Corrupted ciphertext: encrypt then corrupt
	encrypted, err := crypto.Encrypt([]byte("data"), testKeyHex)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a byte near the end
	corrupted := make([]byte, len(encrypted))
	copy(corrupted, encrypted)
	corrupted[len(corrupted)-1] ^= 0xff
	_, err = crypto.Decrypt(corrupted, testKeyHex)
	if err == nil {
		t.Error("expected error for corrupted ciphertext, got nil")
	}
}

func TestDecryptLegacyKey(t *testing.T) {
	// Encrypt produces versioned format (version byte prefix).
	// To test legacy key fallback, we need unversioned ciphertext.
	// The versioned path only uses the primary key.
	// Legacy fallback is for unversioned (no version byte) data.
	// We simulate legacy data by stripping the version byte from encrypted output.
	plaintext := []byte("legacy secret")
	encrypted, err := crypto.Encrypt(plaintext, wrongKeyHex)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Strip version byte to simulate legacy format
	legacyData := encrypted[1:]

	// Decrypt with testKeyHex as primary should fail (wrong key)
	_, err = crypto.Decrypt(legacyData, testKeyHex)
	if err == nil {
		t.Fatal("expected error decrypting legacy data with wrong primary key")
	}

	// Decrypt with testKeyHex as primary and wrongKeyHex as legacy should succeed
	decrypted, err := crypto.Decrypt(legacyData, testKeyHex, wrongKeyHex)
	if err != nil {
		t.Fatalf("Decrypt with legacy key: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("legacy roundtrip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestHashPasswordAndCheck(t *testing.T) {
	password := "correct-horse-battery-staple"
	hash, err := crypto.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if err := crypto.CheckPassword(password, hash); err != nil {
		t.Errorf("CheckPassword with correct password: %v", err)
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, err := crypto.HashPassword("right-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := crypto.CheckPassword("wrong-password", hash); err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestPrehashAvoidsTruncation(t *testing.T) {
	// bcrypt silently truncates at 72 bytes. These two passwords share
	// the first 73 bytes and differ only at byte 74. Without prehash,
	// bcrypt would treat them as identical.
	pw1 := strings.Repeat("A", 73) + "1"
	pw2 := strings.Repeat("A", 73) + "2"

	hash1, err := crypto.HashPassword(pw1)
	if err != nil {
		t.Fatalf("HashPassword(pw1): %v", err)
	}

	// pw2 must NOT match hash of pw1
	if err := crypto.CheckPassword(pw2, hash1); err == nil {
		t.Error("CheckPassword(pw2, hash1) succeeded; prehash is not preventing bcrypt truncation")
	}

	// pw1 must still match its own hash
	if err := crypto.CheckPassword(pw1, hash1); err != nil {
		t.Errorf("CheckPassword(pw1, hash1) failed: %v", err)
	}
}

func TestHMACDigestDeterministic(t *testing.T) {
	d1 := crypto.HMACDigest("data", "secret")
	d2 := crypto.HMACDigest("data", "secret")
	if d1 != d2 {
		t.Errorf("HMAC not deterministic: %q != %q", d1, d2)
	}
	if d1 == "" {
		t.Error("HMAC digest is empty")
	}
}

func TestHMACDigestDifferentInput(t *testing.T) {
	d1 := crypto.HMACDigest("input-a", "secret")
	d2 := crypto.HMACDigest("input-b", "secret")
	if d1 == d2 {
		t.Error("different inputs produced same HMAC digest")
	}
}

func TestHMACDigestDifferentSecret(t *testing.T) {
	d1 := crypto.HMACDigest("data", "secret-1")
	d2 := crypto.HMACDigest("data", "secret-2")
	if d1 == d2 {
		t.Error("different secrets produced same HMAC digest")
	}
}

func TestGenerateTokenFormat(t *testing.T) {
	raw, hash, err := crypto.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if raw == "" {
		t.Error("GenerateToken raw is empty")
	}
	if hash == "" {
		t.Error("GenerateToken hash is empty")
	}
	// raw is base64url-encoded 32 bytes = 43 chars (no padding)
	if len(raw) != 43 {
		t.Errorf("GenerateToken raw length: got %d, want 43", len(raw))
	}
	// hash is hex-encoded SHA-256 = 64 chars
	if len(hash) != 64 {
		t.Errorf("GenerateToken hash length: got %d, want 64", len(hash))
	}
}

func TestGenerateTokenUnique(t *testing.T) {
	raw1, _, err := crypto.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken (1): %v", err)
	}
	raw2, _, err := crypto.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken (2): %v", err)
	}
	if raw1 == raw2 {
		t.Error("two GenerateToken calls produced the same raw value")
	}
}

func TestGenerateAPIKeyFormat(t *testing.T) {
	raw, digest, display, err := crypto.GenerateAPIKey("test-secret")
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	// raw should be "ao-" + 32 hex chars = 35 chars
	if !strings.HasPrefix(raw, "ao-") {
		t.Errorf("raw key missing 'ao-' prefix: %q", raw)
	}
	if len(raw) != 35 {
		t.Errorf("raw key length: got %d, want 35", len(raw))
	}
	if digest == "" {
		t.Error("digest is empty")
	}
	// display should be first 6 chars + "********"
	if display != raw[:6]+"********" {
		t.Errorf("display mismatch: got %q, want %q", display, raw[:6]+"********")
	}
}

func TestHashTokenRoundTrip(t *testing.T) {
	raw, expectedHash, err := crypto.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	got, err := crypto.HashToken(raw)
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}
	if got != expectedHash {
		t.Errorf("HashToken mismatch: got %q, want %q", got, expectedHash)
	}
}
