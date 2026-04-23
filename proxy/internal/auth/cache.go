package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthVerifyResult mirrors the response from Processing's POST /internal/auth/verify.
type AuthVerifyResult struct {
	Valid              bool            `json:"valid"`
	Reason             string          `json:"reason,omitempty"`
	APIKeyID           string          `json:"api_key_id,omitempty"`
	OrganizationID     string          `json:"organization_id,omitempty"`
	ProviderType       string          `json:"provider_type,omitempty"`
	ProviderKey        string          `json:"provider_key,omitempty"`
	BaseURL            string          `json:"base_url,omitempty"`
	OrganizationStatus string          `json:"organization_status,omitempty"`
	StoreSpanContent   bool            `json:"store_span_content"`
	MaskingConfig      json.RawMessage `json:"masking_config,omitempty"`
}

// cacheEntry holds a cached auth result with its expiry time and LRU tracking.
type cacheEntry struct {
	result     *AuthVerifyResult
	expiresAt  time.Time
	lastUsedAt time.Time
}

// AuthCache provides a TTL-based in-memory cache for API key verification results.
// On Processing unavailability, it serves stale entries (fail-open) or returns nil.
// Includes a circuit breaker to avoid goroutine buildup during prolonged outages.
type AuthCache struct {
	mu              sync.RWMutex
	entries         map[string]cacheEntry
	ttl             time.Duration
	maxEntries      int
	evictInterval   time.Duration
	processingURL   string
	internalToken   string
	hmacSecret      string
	httpClient      *http.Client

	// Circuit breaker state
	cbMu              sync.Mutex
	cbConsecutiveFail int
	cbOpenUntil       time.Time
}

const (
	cbFailThreshold  = 3               // open circuit after N consecutive failures
	cbCooldown       = 30 * time.Second // reject fast for this duration before retrying
	maxStaleDuration = 5 * time.Minute // max time a stale entry can be served during outage
)

// NewAuthCache creates a new AuthCache. Starts a background goroutine for periodic eviction.
// The eviction goroutine stops when ctx is cancelled.
// evictInterval controls how often expired entries are swept (0 defaults to 60s).
func NewAuthCache(ctx context.Context, processingURL, internalToken, hmacSecret string, ttl time.Duration, httpClient *http.Client, evictInterval time.Duration) *AuthCache {
	if evictInterval <= 0 {
		evictInterval = 60 * time.Second
	}
	c := &AuthCache{
		entries:       make(map[string]cacheEntry),
		ttl:           ttl,
		maxEntries:    10000,
		evictInterval: evictInterval,
		processingURL: processingURL,
		internalToken: internalToken,
		hmacSecret:    hmacSecret,
		httpClient:    httpClient,
	}
	go c.evictLoop(ctx)
	return c
}

// evictLoop periodically removes expired entries from the cache.
// Stops when ctx is cancelled.
func (c *AuthCache) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(c.evictInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, e := range c.entries {
				if now.After(e.expiresAt) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// hmacDigest computes HMAC-SHA256 of data using secret, returning hex-encoded string.
func hmacDigest(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// Lookup checks the cache for a valid auth result for the given raw API key.
// On cache miss, it fetches from Processing. On Processing error with a stale
// valid entry, returns the stale result (fail-open). Invalid/unknown keys are
// never served from stale cache.
func (c *AuthCache) Lookup(ctx context.Context, rawKey string) (*AuthVerifyResult, error) {
	digest := hmacDigest(rawKey, c.hmacSecret)

	// Single write lock for atomic check + LRU update (avoids TOCTOU between RLock/Lock)
	c.mu.Lock()
	entry, found := c.entries[digest]
	fresh := found && time.Now().Before(entry.expiresAt)
	if fresh {
		entry.lastUsedAt = time.Now()
		c.entries[digest] = entry
	}
	c.mu.Unlock()

	if fresh {
		return entry.result, nil
	}

	// Cache miss or expired — fetch from Processing.
	// Circuit breaker: if open, skip the call and serve stale or reject immediately.
	// NOTE: Between the fresh check above and here, evictLoop could delete a stale
	// entry. This is a known benign race — it results in a cache miss (re-fetch from
	// Processing), not data corruption or incorrect auth decisions.
	if c.circuitOpen() {
		if found && entry.result != nil && entry.result.Valid && time.Since(entry.expiresAt) < maxStaleDuration {
			return entry.result, nil
		}
		return nil, fmt.Errorf("auth cache: circuit breaker open, processing unavailable")
	}

	fetchCtx, fetchCancel := context.WithTimeout(ctx, 10*time.Second)
	defer fetchCancel()
	result, err := c.fetchFromProcessing(fetchCtx, digest)
	if err != nil {
		c.recordFailure()
		// Fail-open: only return stale entry if it was previously valid and not too old.
		// Invalid or unknown keys must not be served from stale cache.
		if found && entry.result != nil && entry.result.Valid && time.Since(entry.expiresAt) < maxStaleDuration {
			return entry.result, nil
		}
		return nil, fmt.Errorf("auth cache: processing unavailable: %w", err)
	}
	c.recordSuccess()

	// Only cache valid results (invalid keys should be re-verified)
	if result.Valid {
		c.mu.Lock()
		// Enforce max size: if at capacity, evict expired entries first
		if len(c.entries) >= c.maxEntries {
			now := time.Now()
			for k, e := range c.entries {
				if now.After(e.expiresAt) {
					delete(c.entries, k)
				}
			}
		}
		// If still at capacity after expired eviction, evict least recently used
		if len(c.entries) >= c.maxEntries {
			slog.Warn("auth cache at capacity, evicting least recently used", "max_entries", c.maxEntries)
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, e := range c.entries {
				if first || e.lastUsedAt.Before(oldestTime) {
					oldestKey = k
					oldestTime = e.lastUsedAt
					first = false
				}
			}
			if oldestKey != "" {
				delete(c.entries, oldestKey)
			}
		}
		now := time.Now()
		c.entries[digest] = cacheEntry{
			result:     result,
			expiresAt:  now.Add(c.ttl),
			lastUsedAt: now,
		}
		c.mu.Unlock()
	}

	return result, nil
}

// circuitOpen returns true if the circuit breaker is open (Processing considered down).
func (c *AuthCache) circuitOpen() bool {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	return c.cbConsecutiveFail >= cbFailThreshold && time.Now().Before(c.cbOpenUntil)
}

// recordFailure increments the consecutive failure counter and opens the circuit if threshold reached.
func (c *AuthCache) recordFailure() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.cbConsecutiveFail++
	if c.cbConsecutiveFail >= cbFailThreshold {
		c.cbOpenUntil = time.Now().Add(cbCooldown)
		if c.cbConsecutiveFail == cbFailThreshold {
			slog.Warn("auth cache circuit breaker opened", "consecutive_failures", cbFailThreshold, "cooldown", cbCooldown)
		}
	}
}

// recordSuccess resets the circuit breaker.
func (c *AuthCache) recordSuccess() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	if c.cbConsecutiveFail > 0 {
		c.cbConsecutiveFail = 0
		c.cbOpenUntil = time.Time{}
	}
}

// verifyRequest is the JSON body for POST /internal/auth/verify.
type verifyRequest struct {
	KeyDigest string `json:"key_digest"`
}

// fetchFromProcessing calls POST /internal/auth/verify on the Processing service.
func (c *AuthCache) fetchFromProcessing(ctx context.Context, digest string) (*AuthVerifyResult, error) {
	body, err := json.Marshal(verifyRequest{KeyDigest: digest})
	if err != nil {
		return nil, fmt.Errorf("auth cache: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.processingURL, "/")+"/internal/auth/verify", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth cache: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth cache: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth cache: unexpected status %d", resp.StatusCode)
	}

	var result AuthVerifyResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("auth cache: decode response: %w", err)
	}

	// Validate that valid results have required fields for proxy routing.
	if result.Valid && (result.ProviderType == "" || result.BaseURL == "" || result.ProviderKey == "") {
		return nil, fmt.Errorf("auth cache: server returned valid result with missing provider_type, base_url, or provider_key")
	}

	return &result, nil
}
