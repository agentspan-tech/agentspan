package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProcessingConfig struct {
	DatabaseURL    string
	JWTSecret      string
	HMACSecret     string
	EncryptionKey  string
	InternalToken  string
	AppBaseURL     string
	Port           string
	DeploymentMode string // "cloud", "self_host"
	JWTTTLDays     int
	AllowedOrigins string
	TrustedProxies []string

	// SMTP configuration (optional — omit SMTP_HOST to use copyable links instead of email)
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// Internal API IP allowlist (optional — omit to allow any IP with valid token)
	InternalAllowedIPs []string

	// Data retention (optional — 0 = keep forever)
	DataRetentionDays int

	// Export row limit (optional — 0 = use default of 100000)
	ExportRowLimit int

	// Skip email verification (optional — set to "1" to auto-verify users on registration)
	SkipEmailVerification bool

	// LLM configuration for intelligence pipeline (optional — omit PROCESSING_LLM_BASE_URL to disable)
	LLMAPIKey  string
	LLMModel   string
	LLMBaseURL string

	// Application version (optional — surfaced in the UI footer for deployment identification)
	AppVersion string
}

func LoadProcessing() (*ProcessingConfig, error) {
	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	jwtSecret, err := requireEnv("JWT_SECRET")
	if err != nil {
		return nil, err
	}
	hmacSecret, err := requireEnv("HMAC_SECRET")
	if err != nil {
		return nil, err
	}
	encryptionKey, err := requireEnv("ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	internalToken, err := requireEnv("INTERNAL_TOKEN")
	if err != nil {
		return nil, err
	}
	appBaseURL, err := requireEnv("APP_BASE_URL")
	if err != nil {
		return nil, err
	}
	cfg := &ProcessingConfig{
		DatabaseURL:    databaseURL,
		JWTSecret:      jwtSecret,
		HMACSecret:     hmacSecret,
		EncryptionKey:  encryptionKey,
		InternalToken:  internalToken,
		AppBaseURL:     appBaseURL,
		Port:           getEnvOrDefault("PROCESSING_PORT", getEnvOrDefault("PORT", "8081")),
		DeploymentMode: getEnvOrDefault("DEPLOYMENT_MODE", "cloud"),
		JWTTTLDays:     getIntOrDefault("JWT_TTL_DAYS", 30),
		AllowedOrigins: getEnvOrDefault("ALLOWED_ORIGINS", ""),
	}
	if tp := getEnvOrDefault("TRUSTED_PROXIES", ""); tp != "" {
		for _, p := range strings.Split(tp, ",") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.TrustedProxies = append(cfg.TrustedProxies, p)
			}
		}
	}

	cfg.SMTPHost = getEnvOrDefault("SMTP_HOST", "")
	cfg.SMTPPort = getIntOrDefault("SMTP_PORT", 587)
	cfg.SMTPUser = getEnvOrDefault("SMTP_USER", "")
	cfg.SMTPPass = getEnvOrDefault("SMTP_PASS", "")
	cfg.SMTPFrom = getEnvOrDefault("SMTP_FROM", "")

	if ips := getEnvOrDefault("INTERNAL_ALLOWED_IPS", ""); ips != "" {
		for _, ip := range strings.Split(ips, ",") {
			if ip = strings.TrimSpace(ip); ip != "" {
				cfg.InternalAllowedIPs = append(cfg.InternalAllowedIPs, ip)
			}
		}
	}

	cfg.SkipEmailVerification = getEnvOrDefault("SKIP_EMAIL_VERIFICATION", "") == "1"
	cfg.DataRetentionDays = getIntOrDefault("DATA_RETENTION_DAYS", 0)
	cfg.ExportRowLimit = getIntOrDefault("EXPORT_ROW_LIMIT", 100000)

	cfg.LLMAPIKey = getEnvOrDefault("PROCESSING_LLM_API_KEY", "")
	cfg.LLMModel = getEnvOrDefault("PROCESSING_LLM_MODEL", "")
	cfg.LLMBaseURL = getEnvOrDefault("PROCESSING_LLM_BASE_URL", "")

	cfg.AppVersion = getEnvOrDefault("APP_VERSION", "")

	// Warn about insecure database connection
	if strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
		slog.Warn("DATABASE_URL uses sslmode=disable — use sslmode=require for production deployments")
	}

	// Validate secret strength — weak secrets enable brute-force attacks
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters (got %d)", len(cfg.JWTSecret))
	}
	if len(cfg.HMACSecret) < 32 {
		return nil, fmt.Errorf("HMAC_SECRET must be at least 32 characters (got %d)", len(cfg.HMACSecret))
	}
	if len(cfg.EncryptionKey) != 64 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 64 hex characters (got %d)", len(cfg.EncryptionKey))
	}
	if _, err := hex.DecodeString(cfg.EncryptionKey); err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}
	if len(cfg.InternalToken) < 32 {
		return nil, fmt.Errorf("INTERNAL_TOKEN must be at least 32 characters (got %d)", len(cfg.InternalToken))
	}

	// validate deployment mode
	switch cfg.DeploymentMode {
	case "cloud", "self_host":
		// valid
	default:
		return nil, fmt.Errorf("invalid DEPLOYMENT_MODE value %q: must be cloud or self_host", cfg.DeploymentMode)
	}
	return cfg, nil
}

func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required env var %s is not set", key)
	}
	return v, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// JWTTTLDuration returns the JWT TTL as a time.Duration.
func (c *ProcessingConfig) JWTTTLDuration() time.Duration {
	return time.Duration(c.JWTTTLDays) * 24 * time.Hour
}
