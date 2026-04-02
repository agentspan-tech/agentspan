package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testHMACDigest(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestHMACDigest(t *testing.T) {
	// Known test vector: HMAC-SHA256 of "as-abc123" with secret "testsecret"
	digest := hmacDigest("as-abc123", "testsecret")
	expected := testHMACDigest("as-abc123", "testsecret")
	if digest != expected {
		t.Errorf("hmacDigest mismatch: got %s, want %s", digest, expected)
	}
	// Verify it's a 64-char hex string (SHA256 output)
	if len(digest) != 64 {
		t.Errorf("digest length: got %d, want 64", len(digest))
	}
}

func TestCacheHit(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "key-1", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	// Pre-populate cache manually
	digest := hmacDigest("as-mykey", "secret")
	cache.mu.Lock()
	cache.entries[digest] = cacheEntry{
		result:    &AuthVerifyResult{Valid: true, APIKeyID: "cached-id"},
		expiresAt: time.Now().Add(10 * time.Second),
	}
	cache.mu.Unlock()

	result, err := cache.Lookup(context.Background(), "as-mykey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.APIKeyID != "cached-id" {
		t.Errorf("expected cached-id, got %+v", result)
	}
	if callCount.Load() != 0 {
		t.Errorf("expected 0 server calls, got %d", callCount.Load())
	}
}

func TestCacheMiss(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "fetched-id", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	result, err := cache.Lookup(context.Background(), "as-newkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.APIKeyID != "fetched-id" {
		t.Errorf("expected fetched-id, got %+v", result)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 server call, got %d", callCount.Load())
	}

	// Second lookup should be cached
	result, err = cache.Lookup(context.Background(), "as-newkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.APIKeyID != "fetched-id" {
		t.Errorf("expected fetched-id from cache, got %+v", result)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected still 1 server call after cache hit, got %d", callCount.Load())
	}
}

func TestCacheExpiry(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "id", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// First call populates cache
	_, err := cache.Lookup(context.Background(), "as-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", callCount.Load())
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Second call should re-fetch
	_, err = cache.Lookup(context.Background(), "as-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls after expiry, got %d", callCount.Load())
	}
}

func TestFailOpenStaleEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "stale-id", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// Populate cache
	_, err := cache.Lookup(context.Background(), "as-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shut down server and wait for expiry
	srv.Close()
	time.Sleep(100 * time.Millisecond)

	// Should return stale entry (fail-open)
	result, err := cache.Lookup(context.Background(), "as-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.APIKeyID != "stale-id" {
		t.Errorf("expected stale-id, got %+v", result)
	}
}

func TestFailOpenNoEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	srv.Close() // Immediately close

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, &http.Client{Timeout: 100 * time.Millisecond}, 0)

	result, err := cache.Lookup(context.Background(), "as-key")
	// Fail-open means we serve stale cache entries during outages, but if there's
	// no cached entry at all we must reject — we can't authenticate an unknown key.
	if err == nil {
		t.Fatal("expected error when processing is down with no stale entry")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestInvalidKeyNotCached(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: false, Reason: "invalid_key"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	// First call
	result, err := cache.Lookup(context.Background(), "as-badkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Valid {
		t.Errorf("expected valid=false, got %+v", result)
	}

	// Second call should call server again (not cached)
	_, err = cache.Lookup(context.Background(), "as-badkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (invalid not cached), got %d", callCount.Load())
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError) // triggers failure
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// Trip the circuit breaker with cbFailThreshold consecutive failures
	for i := 0; i < cbFailThreshold; i++ {
		_, _ = cache.Lookup(context.Background(), "as-key"+string(rune('0'+i)))
	}

	// Circuit should now be open — next call should fail fast without hitting server
	beforeCount := callCount.Load()
	_, err := cache.Lookup(context.Background(), "as-newkey")
	if err == nil {
		t.Fatal("expected error when circuit breaker is open")
	}
	afterCount := callCount.Load()
	if afterCount != beforeCount {
		t.Errorf("circuit breaker should have prevented server call, but got %d additional calls", afterCount-beforeCount)
	}
}

func TestCircuitBreakerServesStaleWhenOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "key-1", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// Populate cache
	_, err := cache.Lookup(context.Background(), "as-mykey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close server and wait for entry to expire
	srv.Close()
	time.Sleep(100 * time.Millisecond)

	// Trip circuit breaker
	for i := 0; i < cbFailThreshold; i++ {
		cache.recordFailure()
	}

	// Stale entry should still be served when circuit is open
	result, err := cache.Lookup(context.Background(), "as-mykey")
	if err != nil {
		t.Fatalf("expected stale result from circuit breaker, got error: %v", err)
	}
	if result == nil || result.APIKeyID != "key-1" {
		t.Errorf("expected stale key-1, got %+v", result)
	}
}

func TestCircuitBreakerResets(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "key-1", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	// Trip circuit breaker
	for i := 0; i < cbFailThreshold; i++ {
		cache.recordFailure()
	}

	// Reset it
	cache.recordSuccess()

	// Should be able to make calls again
	if cache.circuitOpen() {
		t.Error("circuit should be closed after recordSuccess")
	}
}

func TestEvictLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "id", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()
	defer cancel()

	// Very short TTL and evict interval
	cache := NewAuthCache(ctx, srv.URL, "tok", "secret", 10*time.Millisecond, srv.Client(), 50*time.Millisecond)

	// Populate cache
	_, err := cache.Lookup(context.Background(), "as-evictme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify entry exists
	cache.mu.RLock()
	beforeLen := len(cache.entries)
	cache.mu.RUnlock()
	if beforeLen != 1 {
		t.Fatalf("expected 1 entry, got %d", beforeLen)
	}

	// Wait for entry to expire and eviction to run
	time.Sleep(200 * time.Millisecond)

	cache.mu.RLock()
	afterLen := len(cache.entries)
	cache.mu.RUnlock()
	if afterLen != 0 {
		t.Errorf("expected 0 entries after eviction, got %d", afterLen)
	}
}

func TestLRUEviction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "id", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Minute, srv.Client(), 0)
	cache.maxEntries = 2 // Override for test

	// Fill to capacity
	_, _ = cache.Lookup(context.Background(), "as-key1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	time.Sleep(5 * time.Millisecond) // ensure different lastUsedAt
	_, _ = cache.Lookup(context.Background(), "as-key2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	time.Sleep(5 * time.Millisecond)

	// This should trigger LRU eviction of key1 (oldest lastUsedAt)
	_, _ = cache.Lookup(context.Background(), "as-key3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	cache.mu.RLock()
	count := len(cache.entries)
	cache.mu.RUnlock()
	if count > 2 {
		t.Errorf("expected at most 2 entries after LRU eviction, got %d", count)
	}
}

func TestFetchFromProcessing_MissingProviderFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Valid=true but missing required fields
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "key-1"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	_, err := cache.Lookup(context.Background(), "as-abcdef1234567890abcdef1234567890")
	if err == nil {
		t.Fatal("expected error for valid result with missing provider fields")
	}
}

func TestFetchFromProcessing_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 10*time.Second, srv.Client(), 0)

	_, err := cache.Lookup(context.Background(), "as-abcdef1234567890abcdef1234567890")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestInternalTokenSent(t *testing.T) {
	var receivedToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Internal-Token")
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))
	defer srv.Close()

	cache := NewAuthCache(context.Background(), srv.URL, "my-secret-token", "secret", 10*time.Second, srv.Client(), 0)
	_, err := cache.Lookup(context.Background(), "as-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedToken != "my-secret-token" {
		t.Errorf("expected X-Internal-Token 'my-secret-token', got '%s'", receivedToken)
	}
}

func TestFailOpenStaleInvalidEntry(t *testing.T) {
	// First, populate cache with a VALID key so it gets cached
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Return invalid result
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: false, Reason: "invalid_key"})
	}))

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// Lookup the key — server returns invalid, so it won't be cached
	result, err := cache.Lookup(context.Background(), "as-badkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid result")
	}

	// Manually insert a stale INVALID entry to simulate edge case
	// (e.g., if caching logic changes to cache invalid results too)
	digest := hmacDigest("as-badkey-stale", "secret")
	cache.mu.Lock()
	cache.entries[digest] = cacheEntry{
		result:    &AuthVerifyResult{Valid: false, Reason: "revoked"},
		expiresAt: time.Now().Add(-1 * time.Second), // already expired
	}
	cache.mu.Unlock()

	// Shut down server
	srv.Close()

	// Lookup should NOT return the stale invalid entry — must return error
	result, err = cache.Lookup(context.Background(), "as-badkey-stale")
	if err == nil {
		t.Fatal("expected error: stale invalid key must not be served from cache")
	}
	if result != nil {
		t.Errorf("expected nil result for stale invalid key, got %+v", result)
	}
}

func TestMaxStaleDurationExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthVerifyResult{Valid: true, APIKeyID: "key-1", ProviderType: "openai", BaseURL: "https://api.openai.com", ProviderKey: "sk-test"})
	}))

	cache := NewAuthCache(context.Background(), srv.URL, "tok", "secret", 50*time.Millisecond, srv.Client(), 0)

	// Populate cache
	_, err := cache.Lookup(context.Background(), "as-stalekey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close server
	srv.Close()

	// Manually set the entry's expiresAt to more than maxStaleDuration ago
	digest := hmacDigest("as-stalekey", "secret")
	cache.mu.Lock()
	entry := cache.entries[digest]
	entry.expiresAt = time.Now().Add(-(maxStaleDuration + time.Minute)) // 6 minutes ago
	cache.entries[digest] = entry
	cache.mu.Unlock()

	// Lookup should fail — stale entry is too old
	result, err := cache.Lookup(context.Background(), "as-stalekey")
	if err == nil {
		t.Fatal("expected error: stale entry beyond maxStaleDuration should not be served")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}
