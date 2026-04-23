//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
	"github.com/google/uuid"
)

func TestAPIKeyService_GetAPIKey_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	user := createTestUser(t, ctx, sharedQueries, "get-apikey@example.com", "Get User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Get Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	result, err := svc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-get", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	item, err := svc.GetAPIKey(ctx, org.ID, result.ID)
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if item.ID != result.ID {
		t.Errorf("ID mismatch")
	}
	if item.Name != "Agent" {
		t.Errorf("expected Agent, got %s", item.Name)
	}
	if item.ProviderType != "openai" {
		t.Errorf("expected openai, got %s", item.ProviderType)
	}
	if !item.Active {
		t.Error("expected active")
	}
}

func TestAPIKeyService_GetAPIKey_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	user := createTestUser(t, ctx, sharedQueries, "nf-apikey@example.com", "NF User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "NF Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = svc.GetAPIKey(ctx, org.ID, uuid.New())
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

func TestAPIKeyService_GetAPIKey_WrongOrg(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user1 := createTestUser(t, ctx, sharedQueries, "org1@example.com", "Org1 User")
	org1, err := orgSvc.CreateOrganization(ctx, user1.ID, "Org 1")
	if err != nil {
		t.Fatalf("create org1: %v", err)
	}
	result, err := svc.CreateAPIKey(ctx, org1.ID, "Agent", "openai", "sk-wo", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	user2 := createTestUser(t, ctx, sharedQueries, "org2@example.com", "Org2 User")
	org2, err := orgSvc.CreateOrganization(ctx, user2.ID, "Org 2")
	if err != nil {
		t.Fatalf("create org2: %v", err)
	}

	// Looking up with different org should fail
	_, err = svc.GetAPIKey(ctx, org2.ID, result.ID)
	if err == nil {
		t.Fatal("expected cross-org lookup to fail")
	}
}
