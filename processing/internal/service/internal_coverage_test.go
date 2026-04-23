//go:build integration

package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

func hmacDigestHex(s, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(s))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestInternalService_VerifyAPIKey_InvalidKey(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()
	svc := service.NewInternalService(sharedQueries, sharedPool, "test-hmac-secret", encKey, h)

	result, err := svc.VerifyAPIKey(ctx, "nonexistent-digest")
	if err != nil {
		t.Fatalf("VerifyAPIKey: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for nonexistent digest")
	}
	if result.Reason != "invalid_key" {
		t.Errorf("expected reason invalid_key, got %s", result.Reason)
	}
}

func TestInternalService_VerifyAPIKey_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	user := createTestUser(t, ctx, sharedQueries, "verify@example.com", "Verify U")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Verify Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	result, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-provider", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Compute digest of the generated raw AgentOrbit key
	digest := hmacDigestHex(result.RawKey, "test-hmac-secret")

	h := hub.New()
	svc := service.NewInternalService(sharedQueries, sharedPool, "test-hmac-secret", encKey, h)
	vr, err := svc.VerifyAPIKey(ctx, digest)
	if err != nil {
		t.Fatalf("VerifyAPIKey: %v", err)
	}
	if !vr.Valid {
		t.Fatalf("expected Valid=true, got %+v", vr)
	}
	if vr.ProviderType != "openai" {
		t.Errorf("expected openai, got %s", vr.ProviderType)
	}
	if vr.ProviderKey != "sk-provider" {
		t.Errorf("expected provider key sk-provider, got %s", vr.ProviderKey)
	}
	if vr.BaseURL == "" {
		t.Error("expected non-empty base URL")
	}
}

func TestInternalService_VerifyAPIKey_OrgPendingDeletion(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	user := createTestUser(t, ctx, sharedQueries, "pending@example.com", "Pending U")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Pending Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	result, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-p", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Mark org as pending_deletion
	_, err = sharedPool.Exec(ctx, "UPDATE organizations SET status='pending_deletion', deletion_scheduled_at=$1 WHERE id=$2", time.Now().Add(14*24*time.Hour), org.ID)
	if err != nil {
		t.Fatalf("update org: %v", err)
	}

	digest := hmacDigestHex(result.RawKey, "test-hmac-secret")
	h := hub.New()
	svc := service.NewInternalService(sharedQueries, sharedPool, "test-hmac-secret", encKey, h)

	vr, err := svc.VerifyAPIKey(ctx, digest)
	if err != nil {
		t.Fatalf("VerifyAPIKey: %v", err)
	}
	if vr.Valid {
		t.Error("expected Valid=false for pending_deletion org")
	}
	if vr.Reason != "org_pending_deletion" {
		t.Errorf("expected org_pending_deletion, got %s", vr.Reason)
	}
}

