package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProxyConfig struct {
	ProcessingURL   string        // URL of the processing service
	InternalToken   string        // X-Internal-Token shared secret
	Port            string        // port to listen on (default "8080")
	ProviderTimeout time.Duration // timeout for provider requests (default 120s)
	HMACSecret      string        // HMAC-SHA256 secret for API key digest computation
	AuthCacheTTL    time.Duration // TTL for auth cache entries (default 30s)
	SpanBufferSize         int           // buffered channel size for span dispatch (default 1000)
	SpanSendTimeout        time.Duration // timeout for individual span send to Processing (default 10s)
	DefaultAnthropicVersion string       // default anthropic-version header (default "2024-10-22")
	AllowPrivateProviderIPs bool         // allow provider URLs pointing to private IPs (default false)
	AllowPlaintextInternal  bool         // allow HTTP (non-TLS) PROCESSING_URL (default false)
	PerKeyRateLimit         int          // max requests per API key per minute (0 = disabled, default 120)
	CacheEvictInterval      time.Duration // interval for cache eviction sweep (default 60s)
	DrainTimeout            time.Duration // timeout for draining spans on shutdown (default 5s)
	SpanWorkers             int           // number of concurrent span dispatch workers (default 3)
}

func LoadProxy() (*ProxyConfig, error) {
	var missing []string
	requireEnv := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	port := getEnvOrDefault("PROXY_PORT", getEnvOrDefault("PORT", "8080"))

	cfg := &ProxyConfig{
		ProcessingURL:   requireEnv("PROCESSING_URL"),
		InternalToken:   requireEnv("INTERNAL_TOKEN"),
		HMACSecret:      requireEnv("HMAC_SECRET"),
		Port:            port,
		ProviderTimeout: time.Duration(getIntOrDefault("PROVIDER_TIMEOUT_SECONDS", 120)) * time.Second,
		AuthCacheTTL:    time.Duration(getIntOrDefault("AUTH_CACHE_TTL_SECONDS", 30)) * time.Second,
		SpanBufferSize:         getIntOrDefault("SPAN_BUFFER_SIZE", 1000),
		SpanSendTimeout:        time.Duration(getIntOrDefault("SPAN_SEND_TIMEOUT_SECONDS", 10)) * time.Second,
		DefaultAnthropicVersion: getEnvOrDefault("DEFAULT_ANTHROPIC_VERSION", "2024-10-22"),
		AllowPrivateProviderIPs: getEnvOrDefault("ALLOW_PRIVATE_PROVIDER_IPS", "") == "true",
		AllowPlaintextInternal:  getEnvOrDefault("ALLOW_PLAINTEXT_INTERNAL", "") == "true",
		PerKeyRateLimit:         getNonNegativeIntOrDefault("PER_KEY_RATE_LIMIT", 120),
		CacheEvictInterval:      time.Duration(getIntOrDefault("CACHE_EVICT_INTERVAL_SECONDS", 60)) * time.Second,
		DrainTimeout:            time.Duration(getIntOrDefault("DRAIN_TIMEOUT_SECONDS", 5)) * time.Second,
		SpanWorkers:             getIntOrDefault("SPAN_WORKERS", 3),
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("required env vars not set: %s", strings.Join(missing, ", "))
	}

	// Validate secret strength — weak secrets enable brute-force attacks
	if len(cfg.HMACSecret) < 32 {
		return nil, fmt.Errorf("HMAC_SECRET must be at least 32 characters (got %d)", len(cfg.HMACSecret))
	}
	if len(cfg.InternalToken) < 32 {
		return nil, fmt.Errorf("INTERNAL_TOKEN must be at least 32 characters (got %d)", len(cfg.InternalToken))
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return nil, fmt.Errorf("invalid port %q: must be a number between 1 and 65535", port)
	}

	const maxBufferSize = 100000
	if cfg.SpanBufferSize > maxBufferSize {
		slog.Warn("SPAN_BUFFER_SIZE exceeds max, capping", "value", cfg.SpanBufferSize, "max", maxBufferSize)
		cfg.SpanBufferSize = maxBufferSize
	}

	if !strings.HasPrefix(cfg.ProcessingURL, "https://") {
		if !cfg.AllowPlaintextInternal {
			return nil, fmt.Errorf("PROCESSING_URL uses plain HTTP — provider API keys would transit in cleartext. Set ALLOW_PLAINTEXT_INTERNAL=true to override (e.g. Docker networking)")
		}
		slog.Warn("PROCESSING_URL uses plain HTTP (ALLOW_PLAINTEXT_INTERNAL=true) — provider API keys transit in cleartext")
	}

	return cfg, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getNonNegativeIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", def)
			return def
		}
		if n < 0 {
			slog.Warn("negative env var value, using default", "key", key, "value", n, "default", def)
			return def
		}
		return n
	}
	return def
}

func getIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", def)
			return def
		}
		if n < 1 {
			slog.Warn("value must be >= 1, using default", "key", key, "value", n, "default", def)
			return def
		}
		return n
	}
	return def
}
