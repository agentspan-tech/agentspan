//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
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
	if !strings.HasPrefix(result.RawKey, "ao-") {
		t.Errorf("expected raw key to start with 'ao-', got %s", result.RawKey[:6])
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

// mockOpenAIServer creates an httptest server that responds like the OpenAI API.
func mockOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Everything is connected and working!"}},
			},
		})
	}))
}

// mockAnthropicServer creates an httptest server that responds like the Anthropic API.
func mockAnthropicServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"text": "Connection confirmed! Everything is working."},
			},
		})
	}))
}

func TestAPIKeyService_TestKey_OpenAICompat(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))

	// Start mock provider server.
	mockProvider := mockOpenAIServer(t)
	defer mockProvider.Close()

	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)

	// Wire InternalService so span gets recorded.
	h := hub.New()
	internalSvc := service.NewInternalService(queries, pool, "test-hmac-secret-32chars-minimum!", encKey, h)
	svc.SetInternalService(internalSvc)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-openai@example.com", "TestKey User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey OpenAI Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create key pointing at mock provider.
	baseURL := mockProvider.URL
	result, err := svc.CreateAPIKey(ctx, org.ID, "Test Agent", "openai", "sk-test-openai", &baseURL)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Test the key — should auto-select gpt-4o-mini.
	testResult, err := svc.TestAPIKey(ctx, org.ID, result.ID, "")
	if err != nil {
		t.Fatalf("test key: %v", err)
	}
	if !testResult.Success {
		t.Error("expected success=true")
	}
	if testResult.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got '%s'", testResult.Model)
	}
	if testResult.Response == "" {
		t.Error("expected non-empty response")
	}
}

func TestAPIKeyService_TestKey_Anthropic(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))

	mockProvider := mockAnthropicServer(t)
	defer mockProvider.Close()

	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)
	h := hub.New()
	internalSvc := service.NewInternalService(queries, pool, "test-hmac-secret-32chars-minimum!", encKey, h)
	svc.SetInternalService(internalSvc)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-anthropic@example.com", "TestKey Anthropic User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey Anthropic Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	baseURL := mockProvider.URL
	result, err := svc.CreateAPIKey(ctx, org.ID, "Anthropic Agent", "anthropic", "sk-ant-test", &baseURL)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	testResult, err := svc.TestAPIKey(ctx, org.ID, result.ID, "")
	if err != nil {
		t.Fatalf("test key: %v", err)
	}
	if !testResult.Success {
		t.Error("expected success=true")
	}
	if testResult.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model 'claude-haiku-4-5-20251001', got '%s'", testResult.Model)
	}
}

func TestAPIKeyService_TestKey_CustomProviderRequiresModel(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-custom@example.com", "Custom User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey Custom Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	baseURL := "http://localhost:9999"
	result, err := svc.CreateAPIKey(ctx, org.ID, "Custom Agent", "custom", "sk-custom", &baseURL)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Test without model — should fail.
	_, err = svc.TestAPIKey(ctx, org.ID, result.ID, "")
	if err == nil {
		t.Fatal("expected error when testing custom provider without model")
	}
	var se *service.ServiceError
	if !errors.As(err, &se) || se.Code != "model_required" {
		t.Errorf("expected ServiceError with code 'model_required', got: %v", err)
	}
}

func TestAPIKeyService_TestKey_InactiveKeyRejected(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))
	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-inactive@example.com", "Inactive User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey Inactive Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	result, err := svc.CreateAPIKey(ctx, org.ID, "Inactive Agent", "openai", "sk-inactive", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Deactivate the key.
	err = svc.DeactivateAPIKey(ctx, org.ID, result.ID)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// Test should fail with key_inactive.
	_, err = svc.TestAPIKey(ctx, org.ID, result.ID, "")
	if err == nil {
		t.Fatal("expected error when testing inactive key")
	}
	var se *service.ServiceError
	if !errors.As(err, &se) || se.Code != "key_inactive" {
		t.Errorf("expected ServiceError with code 'key_inactive', got: %v", err)
	}
}

func TestAPIKeyService_TestKey_ProviderError(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))

	// Mock provider that returns 401.
	mockProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer mockProvider.Close()

	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-err@example.com", "Err User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey Err Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	baseURL := mockProvider.URL
	result, err := svc.CreateAPIKey(ctx, org.ID, "Err Agent", "openai", "sk-invalid", &baseURL)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	_, err = svc.TestAPIKey(ctx, org.ID, result.ID, "")
	if err == nil {
		t.Fatal("expected error from provider")
	}
	var se *service.ServiceError
	if !errors.As(err, &se) || se.Code != "provider_error" {
		t.Errorf("expected ServiceError with code 'provider_error', got: %v", err)
	}
}

func TestAPIKeyService_TestKey_WithCustomModel(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()

	encKey := hex.EncodeToString(make([]byte, 32))

	mockProvider := mockOpenAIServer(t)
	defer mockProvider.Close()

	svc := service.NewAPIKeyService(queries, "test-hmac-secret-32chars-minimum!", encKey)
	h := hub.New()
	internalSvc := service.NewInternalService(queries, pool, "test-hmac-secret-32chars-minimum!", encKey, h)
	svc.SetInternalService(internalSvc)

	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "testkey-model@example.com", "Model User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "TestKey Model Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	baseURL := mockProvider.URL
	result, err := svc.CreateAPIKey(ctx, org.ID, "Custom Model Agent", "custom", "sk-custom-model", &baseURL)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Test with explicit model.
	testResult, err := svc.TestAPIKey(ctx, org.ID, result.ID, "my-custom-model")
	if err != nil {
		t.Fatalf("test key: %v", err)
	}
	if testResult.Model != "my-custom-model" {
		t.Errorf("expected model 'my-custom-model', got '%s'", testResult.Model)
	}
}
