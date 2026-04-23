package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/auth"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/masking"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/span"
)

// mockAuthServerWithMasking creates an httptest server that returns full auth result
// including store_span_content and masking_config fields.
func mockAuthServerWithMasking(result *auth.AuthVerifyResult) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(result)
		w.Write(data)
	}))
}

// setupMaskingTest sets up a proxy handler with masking-aware auth, returning the handler,
// mock provider server, and span dispatcher for inspecting dispatched spans.
func setupMaskingTest(t *testing.T, providerHandler http.Handler, authResult *auth.AuthVerifyResult) (*ProxyHandler, *httptest.Server, *span.SpanDispatcher) {
	t.Helper()

	provider := httptest.NewServer(providerHandler)
	t.Cleanup(provider.Close)

	if authResult.BaseURL == "" {
		authResult.BaseURL = provider.URL
	}

	authSrv := mockAuthServerWithMasking(authResult)
	t.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 100, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)

	h := NewProxyHandler(context.Background(), cache, dispatcher, 10*time.Second, "2024-10-22", true, 0)
	return h, provider, dispatcher
}

const testPhone = "+79383293838"

// maskedPhone is the expected masked placeholder for testPhone.
const maskedPhone = "[PHONE_1]"
const testInputWithPhone = `{"model":"gpt-4","messages":[{"role":"user","content":"Call me at +79383293838 please"}]}`

func TestProxyMasking_OffMode(t *testing.T) {
	var receivedBody string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"Sure, calling +79383293838"}}]}`)
	})

	h, _, dispatcher := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: true,
		MaskingConfig:    json.RawMessage(`{"mode":"off","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testInputWithPhone))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Provider should receive original body (no masking)
	if !strings.Contains(receivedBody, testPhone) {
		t.Errorf("provider should receive original phone, got: %s", receivedBody)
	}

	// Agent should receive original response
	if !strings.Contains(w.Body.String(), testPhone) {
		t.Errorf("agent should receive original phone in response, got: %s", w.Body.String())
	}

	// Check span
	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if payload.MaskingApplied {
		t.Error("MaskingApplied should be false for off mode")
	}
	if len(payload.MaskingMap) > 0 {
		t.Error("MaskingMap should be empty for off mode")
	}
}

func TestProxyMasking_LLMOnly(t *testing.T) {
	var receivedBody string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		// Echo back the content it received (which should be masked)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// Simulate LLM responding with the masked phone number it received
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"Sure, calling [PHONE_1]"}}]}`)
	})

	h, _, dispatcher := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: true,
		MaskingConfig:    json.RawMessage(`{"mode":"llm_only","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testInputWithPhone))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Provider should receive MASKED body
	if strings.Contains(receivedBody, testPhone) {
		t.Errorf("provider should NOT receive original phone, got: %s", receivedBody)
	}

	// Agent should receive UNMASKED response (masked values replaced back to originals)
	responseBody := w.Body.String()
	if !strings.Contains(responseBody, testPhone) {
		t.Errorf("agent should receive unmasked phone in response, got: %s", responseBody)
	}

	// Check span
	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if !payload.MaskingApplied {
		t.Error("MaskingApplied should be true for llm_only mode")
	}
	// LLM Only: span stores ORIGINAL text
	if !strings.Contains(payload.Input, testPhone) {
		t.Errorf("span input should contain original phone for llm_only, got: %s", payload.Input)
	}
	// Masking map should be populated
	if len(payload.MaskingMap) == 0 {
		t.Error("MaskingMap should be populated for llm_only mode")
	}
	if len(payload.MaskingMap) > 0 {
		entry := payload.MaskingMap[0]
		if entry.OriginalValue != testPhone {
			t.Errorf("masking map original value mismatch: got %q, want %q", entry.OriginalValue, testPhone)
		}
		if entry.MaskType != string(masking.MaskTypePhone) {
			t.Errorf("masking map type mismatch: got %q, want %q", entry.MaskType, masking.MaskTypePhone)
		}
	}
}

func TestProxyMasking_LLMStorage(t *testing.T) {
	var receivedBody string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// LLM responds with the masked phone number it received
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"Sure, calling [PHONE_1]"}}]}`)
	})

	h, _, dispatcher := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: true,
		MaskingConfig:    json.RawMessage(`{"mode":"llm_storage","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testInputWithPhone))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Provider should receive MASKED body
	if strings.Contains(receivedBody, testPhone) {
		t.Errorf("provider should NOT receive original phone, got: %s", receivedBody)
	}

	// Agent should receive UNMASKED response
	responseBody := w.Body.String()
	if !strings.Contains(responseBody, testPhone) {
		t.Errorf("agent should receive unmasked phone in response, got: %s", responseBody)
	}

	// Check span
	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if !payload.MaskingApplied {
		t.Error("MaskingApplied should be true for llm_storage mode")
	}
	// LLM+Storage: span stores MASKED text
	if strings.Contains(payload.Input, testPhone) {
		t.Errorf("span input should NOT contain original phone for llm_storage, got: %s", payload.Input)
	}
	// No masking map for LLM+Storage (PII never persisted)
	if len(payload.MaskingMap) > 0 {
		t.Error("MaskingMap should be empty for llm_storage mode")
	}
}

func TestProxyMasking_MetadataOnly(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"Hello"}}]}`)
	})

	h, _, dispatcher := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: false, // metadata-only mode
		MaskingConfig:    json.RawMessage(`{"mode":"llm_storage","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testInputWithPhone))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check span
	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if payload.MaskingApplied {
		t.Error("MaskingApplied should be false when StoreSpanContent=false")
	}
	if payload.Input != "" {
		t.Errorf("span input should be empty for metadata-only mode, got: %s", payload.Input)
	}
	if payload.Output != "" {
		t.Errorf("span output should be empty for metadata-only mode, got: %s", payload.Output)
	}
}

func TestProxyMasking_LLMStorage_OutputRemasking(t *testing.T) {
	// Verify that when output contains the same phone numbers as input,
	// the stored output is correctly re-masked using request-side entries.
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// LLM responds mentioning the masked phone (711-111-11-11) which will be
		// unmasked to the original for the agent, then re-masked for storage.
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"I will call [PHONE_1] now"}}]}`)
	})

	h, _, dispatcher := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: true,
		MaskingConfig:    json.RawMessage(`{"mode":"llm_storage","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testInputWithPhone))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Agent gets unmasked response
	responseBody := w.Body.String()
	if !strings.Contains(responseBody, testPhone) {
		t.Errorf("agent should receive unmasked phone, got: %s", responseBody)
	}

	// Span output should be re-masked (phone replaced back to masked form)
	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if strings.Contains(payload.Output, testPhone) {
		t.Errorf("span output should NOT contain original phone for llm_storage, got: %s", payload.Output)
	}
	// The output should contain the masked phone pattern
	// which is the re-masked form using request-side entries
	if !strings.Contains(payload.Output, maskedPhone) {
		t.Errorf("span output should contain re-masked phone, got: %s", payload.Output)
	}
}

func TestProxyMasking_FailOpen(t *testing.T) {
	var receivedBody string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"content":"Hello"}}]}`)
	})

	// Test that safeMask returns ok=false on panic without breaking the request.
	// We can't directly cause ApplyMasking to panic in production, but we test
	// the safeMask wrapper directly.
	result, ok := safeMask(nil, &masking.MaskingConfig{Mode: masking.MaskModeLLMOnly, Rules: []masking.MaskingRule{masking.PresetPhoneRule}})
	if !ok {
		t.Error("safeMask with nil input should not panic")
	}
	if result == nil {
		t.Error("safeMask should return a result even for nil input")
	}

	// Also verify the full proxy flow still works with masking config that has
	// no phone numbers in the body (no matches, masking is effectively no-op)
	h, _, _ := setupMaskingTest(t, provider, &auth.AuthVerifyResult{
		Valid:            true,
		APIKeyID:         "key-1",
		OrganizationID:   "org-1",
		ProviderType:     "openai",
		ProviderKey:      "pk-123",
		StoreSpanContent: true,
		MaskingConfig:    json.RawMessage(`{"mode":"llm_only","rules":[{"name":"phone","pattern":"\\+7\\d{10}","builtin":true}]}`),
	})

	// Body with no phone numbers
	noPhoneBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello world"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(noPhoneBody))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Provider should receive the body unchanged
	if receivedBody != noPhoneBody {
		t.Errorf("provider should receive unchanged body, got: %s", receivedBody)
	}
}
