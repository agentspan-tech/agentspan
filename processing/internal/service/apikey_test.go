//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/agentspan/processing/internal/service"
	"github.com/agentspan/processing/internal/testutil"
)

func TestAPIKeyService_Create(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	// Need a valid 64-char hex encryption key
	encKey := hex.EncodeToString(make([]byte, 32))

	svc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	ctx := context.Background()

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "apikey@example.com", "API Key User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "APIKey Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	result, err := svc.CreateAPIKey(ctx, org.ID, "My Agent", "openai", "sk-testkey123", nil)
	if err != nil {
		t.Fatalf("create api key failed: %v", err)
	}
	if !strings.HasPrefix(result.RawKey, "as-") {
		t.Errorf("expected raw key to start with 'as-', got %s", result.RawKey[:6])
	}
	if result.Name != "My Agent" {
		t.Errorf("expected name 'My Agent', got '%s'", result.Name)
	}
	if result.ProviderType != "openai" {
		t.Errorf("expected provider_type 'openai', got '%s'", result.ProviderType)
	}
	if !result.Active {
		t.Error("expected newly created key to be active")
	}
}

func TestAPIKeyService_List(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	ctx := context.Background()

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "apikey-list@example.com", "List User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "List Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	// Create two keys
	_, err = svc.CreateAPIKey(ctx, org.ID, "Agent 1", "openai", "sk-key1", nil)
	if err != nil {
		t.Fatalf("create key 1 failed: %v", err)
	}
	_, err = svc.CreateAPIKey(ctx, org.ID, "Agent 2", "anthropic", "sk-key2", nil)
	if err != nil {
		t.Fatalf("create key 2 failed: %v", err)
	}

	items, err := svc.ListAPIKeys(ctx, org.ID)
	if err != nil {
		t.Fatalf("list api keys failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(items))
	}

	// Display should be masked (no raw key exposed)
	for _, item := range items {
		if strings.Contains(item.Display, "sk-") {
			t.Error("display should not contain provider key")
		}
		if len(item.Display) < 6 {
			t.Error("display should be at least 6 chars")
		}
	}
}

func TestAPIKeyService_Deactivate(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	ctx := context.Background()

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "apikey-deact@example.com", "Deactivate User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Deactivate Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	result, err := svc.CreateAPIKey(ctx, org.ID, "Deactivate Agent", "openai", "sk-deact", nil)
	if err != nil {
		t.Fatalf("create key failed: %v", err)
	}

	err = svc.DeactivateAPIKey(ctx, org.ID, result.ID)
	if err != nil {
		t.Fatalf("deactivate key failed: %v", err)
	}

	// Verify key is in the list but marked inactive
	items, err := svc.ListAPIKeys(ctx, org.ID)
	if err != nil {
		t.Fatalf("list keys failed: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == result.ID {
			found = true
			if item.Active {
				t.Error("expected deactivated key to not be active")
			}
		}
	}
	if !found {
		t.Error("deactivated key should still appear in list (but as inactive)")
	}
}
