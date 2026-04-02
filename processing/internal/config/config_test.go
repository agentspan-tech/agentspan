package config

import (
	"strings"
	"testing"
)

// setRequiredEnv sets the minimum required env vars for LoadProcessing to succeed.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/agentspan?sslmode=disable")
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("HMAC_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("INTERNAL_TOKEN", "cccccccccccccccccccccccccccccccccc")
	t.Setenv("APP_BASE_URL", "http://localhost:3000")
}

func TestLoadProcessing_Success(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if cfg.Port != "8081" {
		t.Errorf("Port = %q, want 8081", cfg.Port)
	}
	if cfg.DeploymentMode != "cloud" {
		t.Errorf("DeploymentMode = %q, want cloud", cfg.DeploymentMode)
	}
	if cfg.JWTTTLDays != 30 {
		t.Errorf("JWTTTLDays = %d, want 30", cfg.JWTTTLDays)
	}
}

func TestLoadProcessing_MissingDatabaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_URL", "")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoadProcessing_MissingJWTSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("JWT_SECRET", "")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestLoadProcessing_ShortJWTSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("JWT_SECRET", "short")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for short JWT_SECRET")
	}
}

func TestLoadProcessing_ShortHMACSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HMAC_SECRET", "short")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for short HMAC_SECRET")
	}
}

func TestLoadProcessing_InvalidEncryptionKeyLength(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENCRYPTION_KEY", "tooshort")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for wrong ENCRYPTION_KEY length")
	}
}

func TestLoadProcessing_InvalidEncryptionKeyHex(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENCRYPTION_KEY", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for invalid hex in ENCRYPTION_KEY")
	}
}

func TestLoadProcessing_ShortInternalToken(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INTERNAL_TOKEN", "short")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for short INTERNAL_TOKEN")
	}
}

func TestLoadProcessing_InvalidDeploymentMode(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DEPLOYMENT_MODE", "enterprise")

	_, err := LoadProcessing()
	if err == nil {
		t.Fatal("expected error for invalid DEPLOYMENT_MODE")
	}
}

func TestLoadProcessing_ValidDeploymentModes(t *testing.T) {
	for _, mode := range []string{"cloud", "self_host"} {
		t.Run(mode, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("DEPLOYMENT_MODE", mode)

			cfg, err := LoadProcessing()
			if err != nil {
				t.Fatalf("unexpected error for deployment mode %q: %v", mode, err)
			}
			if cfg.DeploymentMode != mode {
				t.Errorf("DeploymentMode = %q, want %q", cfg.DeploymentMode, mode)
			}
		})
	}
}

func TestLoadProcessing_TrustedProxies(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1, 10.0.0.2, 10.0.0.3")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.TrustedProxies) != 3 {
		t.Errorf("TrustedProxies len = %d, want 3", len(cfg.TrustedProxies))
	}
}

func TestLoadProcessing_InternalAllowedIPs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INTERNAL_ALLOWED_IPS", "172.16.0.1,172.16.0.2")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.InternalAllowedIPs) != 2 {
		t.Errorf("InternalAllowedIPs len = %d, want 2", len(cfg.InternalAllowedIPs))
	}
}

func TestLoadProcessing_CustomPort(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROCESSING_PORT", "9090")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}

func TestLoadProcessing_SMTPConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASS", "pass")
	t.Setenv("SMTP_FROM", "noreply@example.com")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("SMTPHost = %q", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 465 {
		t.Errorf("SMTPPort = %d, want 465", cfg.SMTPPort)
	}
}

func TestLoadProcessing_LLMConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PROCESSING_LLM_PROVIDER", "anthropic")
	t.Setenv("PROCESSING_LLM_API_KEY", "sk-test")
	t.Setenv("PROCESSING_LLM_MODEL", "claude-3")

	cfg, err := LoadProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMProvider != "anthropic" {
		t.Errorf("LLMProvider = %q", cfg.LLMProvider)
	}
}

func TestJWTTTLDuration(t *testing.T) {
	cfg := &ProcessingConfig{JWTTTLDays: 7}
	d := cfg.JWTTTLDuration()
	if d.Hours() != 168 {
		t.Errorf("JWTTTLDuration = %v, want 168h", d)
	}
}

func TestRequireEnv(t *testing.T) {
	t.Setenv("TEST_REQUIRED", "value")
	v, err := requireEnv("TEST_REQUIRED")
	if err != nil || v != "value" {
		t.Errorf("requireEnv = %q, err = %v", v, err)
	}

	_, err = requireEnv("NONEXISTENT_KEY_12345")
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestLoadProcessing_MissingAppBaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("APP_BASE_URL", "")

	_, err := LoadProcessing()
	if err == nil || !strings.Contains(err.Error(), "APP_BASE_URL") {
		t.Fatalf("expected APP_BASE_URL error, got: %v", err)
	}
}
