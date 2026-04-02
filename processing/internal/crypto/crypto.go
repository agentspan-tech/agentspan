package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"
)

// HMACDigest returns the HMAC-SHA256 hex-encoded digest of data using secret.
func HMACDigest(data string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateAPIKey generates a new AgentSpan API key.
// Returns the raw key (34 chars, "as-" + 32 hex), its HMAC-SHA256 digest, and a display string.
func GenerateAPIKey(hmacSecret string) (raw string, digest string, display string, err error) {
	b := make([]byte, 16)
	if _, err = io.ReadFull(rand.Reader, b); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	raw = "as-" + hex.EncodeToString(b)
	digest = HMACDigest(raw, hmacSecret)
	display = raw[:6] + "********"
	return raw, digest, display, nil
}

// encryptionKeyVersion is the current encryption key version tag.
// All new encryptions use this version. Decrypt supports both versioned and legacy (unversioned) data.
const encryptionKeyVersion byte = 0x01

// Encrypt encrypts plaintext using AES-256-GCM.
// keyHex must be 64 hex characters (32 bytes).
// Returns version_byte || nonce || ciphertext.
func Encrypt(plaintext []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("encrypt: decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encrypt: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encrypt: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encrypt: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("encrypt: generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	// Prepend version byte for key rotation support
	result := make([]byte, 1+len(sealed))
	result[0] = encryptionKeyVersion
	copy(result[1:], sealed)
	return result, nil
}

// Decrypt decrypts AES-256-GCM ciphertext produced by Encrypt.
// keyHex must be 64 hex characters (32 bytes).
// Supports both versioned (version_byte || nonce || ciphertext) and legacy (nonce || ciphertext) formats.
// For key rotation, pass additional old keys via legacyKeys. The primary keyHex is tried first.
func Decrypt(ciphertext []byte, keyHex string, legacyKeys ...string) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("decrypt: empty ciphertext")
	}

	// Check for version prefix
	if ciphertext[0] == encryptionKeyVersion {
		// Versioned format: strip version byte, decrypt with primary key
		return decryptRaw(ciphertext[1:], keyHex)
	}

	// Legacy (unversioned) format: try primary key first, then legacy keys
	plaintext, err := decryptRaw(ciphertext, keyHex)
	if err == nil {
		return plaintext, nil
	}
	for _, lk := range legacyKeys {
		if plaintext, legacyErr := decryptRaw(ciphertext, lk); legacyErr == nil {
			return plaintext, nil
		}
	}
	return nil, err
}

// decryptRaw decrypts AES-256-GCM data in the format nonce || ciphertext.
func decryptRaw(ciphertext []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decrypt: decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("decrypt: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decrypt: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decrypt: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("decrypt: ciphertext too short")
	}
	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// prehashPassword applies SHA-256 before bcrypt to avoid bcrypt's 72-byte silent truncation.
// Two different passwords sharing the same first 72 bytes would otherwise be treated as identical.
func prehashPassword(password string) []byte {
	h := sha256.Sum256([]byte(password))
	// Encode as hex (64 bytes) — well within bcrypt's 72-byte limit.
	dst := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(dst, h[:])
	return dst
}

// HashPassword hashes a password using SHA-256 + bcrypt with DefaultCost.
// SHA-256 pre-hash ensures passwords longer than 72 bytes are not silently truncated.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword(prehashPassword(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// CheckPassword verifies password against a bcrypt hash.
// Applies the same SHA-256 pre-hash used in HashPassword.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), prehashPassword(password))
}

// GenerateToken generates a secure random single-use token.
// Returns the base64url-encoded raw token and its SHA-256 hex hash.
func GenerateToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, b); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256(b)
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

// HashToken computes the SHA-256 hex hash of a raw token (base64url-encoded).
// Used to verify tokens against stored hashes.
func HashToken(raw string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("hash token: decode: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
