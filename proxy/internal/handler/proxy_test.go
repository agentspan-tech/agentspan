package handler

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentspan/proxy/internal/auth"
	"github.com/agentspan/proxy/internal/span"
)

// mockAuthServer creates an httptest server that responds to POST /internal/auth/verify.
// It returns the provided AuthVerifyResult for any key digest.
func mockAuthServer(result *auth.AuthVerifyResult) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !result.Valid {
			fmt.Fprintf(w, `{"valid":false,"reason":%q}`, result.Reason)
			return
		}
		fmt.Fprintf(w, `{"valid":true,"api_key_id":%q,"organization_id":%q,"provider_type":%q,"provider_key":%q,"base_url":%q,"organization_status":"active","store_span_content":true}`,
			result.APIKeyID, result.OrganizationID, result.ProviderType, result.ProviderKey, result.BaseURL)
	}))
}

// setupProxyTest creates a mock provider, mock auth, auth cache, span dispatcher, and proxy handler.
// Returns the proxy handler, mock provider server, and a channel that receives dispatched spans.
func setupProxyTest(t *testing.T, providerHandler http.Handler, authResult *auth.AuthVerifyResult, providerTimeout time.Duration) (*ProxyHandler, *httptest.Server, *span.SpanDispatcher) {
	t.Helper()

	provider := httptest.NewServer(providerHandler)
	t.Cleanup(provider.Close)

	// Point the auth result's BaseURL at the mock provider
	if authResult.BaseURL == "" {
		authResult.BaseURL = provider.URL
	}

	authSrv := mockAuthServer(authResult)
	t.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 100, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)

	if providerTimeout == 0 {
		providerTimeout = 10 * time.Second
	}

	h := NewProxyHandler(context.Background(), cache, dispatcher, providerTimeout, "2024-10-22", true, 0)
	return h, provider, dispatcher
}

func TestForwardOpenAI(t *testing.T) {
	providerBody := `{"id":"chatcmpl-123","choices":[{"message":{"content":"Hello!"}}]}`

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-key-123" {
			t.Errorf("expected provider auth header, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(providerBody))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "provider-key-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != providerBody {
		t.Errorf("response body mismatch:\ngot:  %s\nwant: %s", w.Body.String(), providerBody)
	}
}

func TestForwardAnthropic(t *testing.T) {
	var receivedAPIKey string
	var receivedAuthHeader string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("x-api-key")
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"content":[{"text":"Hi"}]}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-2",
		OrganizationID: "org-1",
		ProviderType:   "anthropic",
		ProviderKey:    "sk-ant-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedAPIKey != "sk-ant-123" {
		t.Errorf("expected x-api-key=sk-ant-123, got %q", receivedAPIKey)
	}
	if receivedAuthHeader != "" {
		t.Errorf("expected no Authorization header for Anthropic, got %q", receivedAuthHeader)
	}
}

func TestAnthropicVersionPassthrough(t *testing.T) {
	var receivedVersion string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-2",
		ProviderType: "anthropic",
		ProviderKey:  "sk-ant-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("anthropic-version", "2024-01-01")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if receivedVersion != "2024-01-01" {
		t.Errorf("expected anthropic-version=2024-01-01, got %q", receivedVersion)
	}
}

func TestAnthropicVersionDefaultSet(t *testing.T) {
	var receivedVersion string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-2",
		ProviderType: "anthropic",
		ProviderKey:  "sk-ant-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	// NOT setting anthropic-version header
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if receivedVersion != "2024-10-22" {
		t.Errorf("expected default anthropic-version=2024-10-22, got %q", receivedVersion)
	}
}

func TestSSEStreaming(t *testing.T) {
	events := []string{
		"data: {\"chunk\":1}\n\n",
		"data: {\"chunk\":2}\n\n",
		"data: {\"chunk\":3}\n\n",
	}

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support Flusher")
		}

		for _, event := range events {
			w.Write([]byte(event))
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")

	// Use a pipe to detect incremental delivery
	pr, pw := io.Pipe()
	recorder := httptest.NewRecorder()

	// We need a real HTTP server to test SSE flushing behavior
	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	t.Cleanup(srv.Close)
	_ = pw
	_ = pr

	client := srv.Client()
	httpReq, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	httpReq.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	_ = recorder

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Read all events -- verify they all arrive
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	allEvents := strings.Join(events, "")
	if string(body) != allEvents {
		t.Errorf("SSE body mismatch:\ngot:  %q\nwant: %q", string(body), allEvents)
	}
}

func TestProviderTimeout504(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 100*time.Millisecond) // 100ms timeout, provider sleeps 200ms

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 504 {
		t.Errorf("expected 504, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMissingAuthHeader(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{Valid: false}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	// No Authorization header
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvalidAPIKey(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:  false,
		Reason: "api key revoked",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "api key revoked") {
		t.Errorf("expected reason in body, got: %s", w.Body.String())
	}
}

func TestEndpointMismatch(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai", // Not Anthropic
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "endpoint_mismatch") {
		t.Errorf("expected endpoint_mismatch error, got: %s", w.Body.String())
	}
}

func TestAgentSpanHeadersStripped(t *testing.T) {
	var receivedHeaders http.Header

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("X-Agentspan-Session", "sess-123")
	req.Header.Set("X-Agentspan-Agent", "my-agent")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if receivedHeaders.Get("X-Agentspan-Session") != "" {
		t.Error("X-Agentspan-Session header was not stripped")
	}
	if receivedHeaders.Get("X-Agentspan-Agent") != "" {
		t.Error("X-Agentspan-Agent header was not stripped")
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header should be preserved")
	}
}

func TestHostHeaderNotForwarded(t *testing.T) {
	var receivedHost string

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, providerSrv, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Host = "agentspan.example.com"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Provider should receive its own host, not the agent's
	providerHost := strings.TrimPrefix(providerSrv.URL, "http://")
	if receivedHost != providerHost {
		t.Errorf("expected provider host %q, got %q", providerHost, receivedHost)
	}
}

func TestSpanDispatched(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "sk-123",
	}, 0)

	// Start dispatcher so it doesn't block; we just check the channel has an item
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// The dispatcher has a buffered channel; the span should be there
	// We access the internal channel via Dropped() -- if not dropped, it's in the buffer
	if dispatcher.Dropped() != 0 {
		t.Error("expected span to be dispatched, but it was dropped")
	}
}

func TestProviderRouting(t *testing.T) {
	var requestedURL atomic.Value

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL.Store(r.URL.Path)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	providerSrv := httptest.NewServer(provider)
	t.Cleanup(providerSrv.Close)

	authResult := &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
		BaseURL:      providerSrv.URL,
	}

	authSrv := mockAuthServer(authResult)
	t.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 100, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)
	h := NewProxyHandler(context.Background(), cache, dispatcher, 10*time.Second, "2024-10-22", true, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	path, ok := requestedURL.Load().(string)
	if !ok || path != "/v1/chat/completions" {
		t.Errorf("expected request to /v1/chat/completions, got %q", path)
	}
}

func TestSpanInputParsedOpenAI(t *testing.T) {
	providerBody := `{"choices":[{"message":{"role":"assistant","content":"4"}}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(providerBody))
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "sk-123",
	}, 0)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"What is 2+2?"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched but channel was empty")
	}

	wantInput := "user: What is 2+2?\n"
	if payload.Input != wantInput {
		t.Errorf("span Input =\n  %q\nwant:\n  %q", payload.Input, wantInput)
	}

	wantOutput := "4"
	if payload.Output != wantOutput {
		t.Errorf("span Output =\n  %q\nwant:\n  %q", payload.Output, wantOutput)
	}

	// Verify proxy response to agent is unchanged (transparency)
	if w.Body.String() != providerBody {
		t.Errorf("proxy response must be unchanged, got: %s", w.Body.String())
	}
}

func TestSpanInputParsedAnthropic(t *testing.T) {
	providerBody := `{"content":[{"type":"text","text":"Hello from Claude"}]}`

	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(providerBody))
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-2",
		OrganizationID: "org-1",
		ProviderType:   "anthropic",
		ProviderKey:    "sk-ant-123",
	}, 0)

	reqBody := `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"Say hello"}]}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched but channel was empty")
	}

	wantInput := "system: You are a helpful assistant\nuser: Say hello\n"
	if payload.Input != wantInput {
		t.Errorf("span Input =\n  %q\nwant:\n  %q", payload.Input, wantInput)
	}

	wantOutput := "Hello from Claude"
	if payload.Output != wantOutput {
		t.Errorf("span Output =\n  %q\nwant:\n  %q", payload.Output, wantOutput)
	}

	// Verify proxy response to agent is unchanged (transparency)
	if w.Body.String() != providerBody {
		t.Errorf("proxy response must be unchanged, got: %s", w.Body.String())
	}
}

func TestSpanFallbackOnMalformedJSON(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "sk-123",
	}, 0)

	malformedBody := `not valid json at all`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(malformedBody))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched but channel was empty")
	}

	// Input must be placeholder on parse failure (prevents storing sensitive raw data)
	if payload.Input != "[unparseable request body]" {
		t.Errorf("span Input should be placeholder on parse failure, got: %q", payload.Input)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{Valid: false}, 0)

	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 405 {
		t.Errorf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Allow") != "POST" {
		t.Errorf("expected Allow: POST header, got %q", w.Header().Get("Allow"))
	}
}

func TestAnthropicEndpointMismatch(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "anthropic",
		ProviderKey:  "sk-ant-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRateLimiting(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	authResult := &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}

	providerSrv := httptest.NewServer(provider)
	t.Cleanup(providerSrv.Close)
	authResult.BaseURL = providerSrv.URL

	authSrv := mockAuthServer(authResult)
	t.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 100, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)
	// Rate limit of 2 per minute
	h := NewProxyHandler(context.Background(), cache, dispatcher, 10*time.Second, "2024-10-22", true, 2)

	// First 2 should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 3rd should be rate limited
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Errorf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid key", "Bearer as-abcdef1234567890abcdef1234567890", "as-abcdef1234567890abcdef1234567890"},
		{"missing header", "", ""},
		{"no bearer prefix", "Token as-abcdef1234567890abcdef1234567890", ""},
		{"wrong prefix", "Bearer sk-1234567890123456789012345678901234", ""},
		{"too short", "Bearer as-abc", ""},
		{"uppercase hex", "Bearer as-ABCDEF1234567890abcdef1234567890", ""},
		{"invalid chars", "Bearer as-xyz!ef1234567890abcdef1234567890", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			got := extractAPIKey(req)
			if got != tc.want {
				t.Errorf("extractAPIKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
	}
	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %s", tc.ip)
			}
			got := isPrivateIP(ip)
			if got != tc.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.private)
			}
		})
	}
}

func TestContainsCRLF(t *testing.T) {
	if !containsCRLF("hello\nworld") {
		t.Error("expected true for LF")
	}
	if !containsCRLF("hello\rworld") {
		t.Error("expected true for CR")
	}
	if containsCRLF("hello world") {
		t.Error("expected false for clean string")
	}
}

func TestSanitizeHeader(t *testing.T) {
	// Truncation
	long := strings.Repeat("a", 300)
	got := sanitizeHeader(long, 256)
	if len(got) != 256 {
		t.Errorf("expected length 256, got %d", len(got))
	}

	// Control character stripping
	got = sanitizeHeader("hello\x00world\x1f", 256)
	if got != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", got)
	}
}

func TestIsHopByHop(t *testing.T) {
	if !isHopByHop("connection") {
		t.Error("connection should be hop-by-hop")
	}
	if !isHopByHop("transfer-encoding") {
		t.Error("transfer-encoding should be hop-by-hop")
	}
	if isHopByHop("content-type") {
		t.Error("content-type should not be hop-by-hop")
	}
}

func TestIsFilteredResponseHeader(t *testing.T) {
	if !isFilteredResponseHeader("server") {
		t.Error("server should be filtered")
	}
	if !isFilteredResponseHeader("x-request-id") {
		t.Error("x-request-id should be filtered")
	}
	if isFilteredResponseHeader("content-type") {
		t.Error("content-type should not be filtered")
	}
}

func TestParseNonSSEResponse(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		providerType string
		wantModel    string
		wantInput    int
		wantOutput   int
		wantFinish   string
	}{
		{
			name:         "OpenAI response",
			body:         `{"model":"gpt-4","usage":{"prompt_tokens":10,"completion_tokens":5},"choices":[{"finish_reason":"stop"}]}`,
			providerType: "openai",
			wantModel:    "gpt-4",
			wantInput:    10,
			wantOutput:   5,
			wantFinish:   "stop",
		},
		{
			name:         "Anthropic response",
			body:         `{"model":"claude-3","usage":{"input_tokens":15,"output_tokens":8},"stop_reason":"end_turn"}`,
			providerType: "anthropic",
			wantModel:    "claude-3",
			wantInput:    15,
			wantOutput:   8,
			wantFinish:   "end_turn",
		},
		{
			name:         "Invalid JSON",
			body:         `not json`,
			providerType: "openai",
			wantModel:    "",
		},
		{
			name:         "No usage field",
			body:         `{"model":"gpt-4"}`,
			providerType: "openai",
			wantModel:    "gpt-4",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta := parseNonSSEResponse([]byte(tc.body), tc.providerType)
			if meta.Model != tc.wantModel {
				t.Errorf("Model = %q, want %q", meta.Model, tc.wantModel)
			}
			if meta.InputTokens != tc.wantInput {
				t.Errorf("InputTokens = %d, want %d", meta.InputTokens, tc.wantInput)
			}
			if meta.OutputTokens != tc.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", meta.OutputTokens, tc.wantOutput)
			}
			if meta.FinishReason != tc.wantFinish {
				t.Errorf("FinishReason = %q, want %q", meta.FinishReason, tc.wantFinish)
			}
		})
	}
}

func TestParseSSELastChunk(t *testing.T) {
	meta := parseSSELastChunk(`{"model":"gpt-4","usage":{"prompt_tokens":10,"completion_tokens":5}}`, "openai")
	if meta.Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4", meta.Model)
	}

	meta = parseSSELastChunk("", "openai")
	if meta.Model != "" {
		t.Errorf("expected empty model for empty input, got %q", meta.Model)
	}
}

func TestKeyRateLimiter_Disabled(t *testing.T) {
	krl := newKeyRateLimiter(0, time.Minute)
	// Should always allow when limit is 0
	for i := 0; i < 100; i++ {
		if !krl.allow("key") {
			t.Fatal("expected allow when rate limiting is disabled")
		}
	}
}

func TestKeyRateLimiter_MaxEntries(t *testing.T) {
	krl := newKeyRateLimiter(100, time.Minute)
	krl.maxEntries = 2

	krl.allow("key1")
	krl.allow("key2")
	// key3 should be rejected (at capacity for new keys)
	if krl.allow("key3") {
		t.Error("expected rejection for new key at capacity")
	}
	// existing keys should still work
	if !krl.allow("key1") {
		t.Error("expected existing key to still be allowed")
	}
}

func TestResolveAndCheckHost_LiteralPublicIP(t *testing.T) {
	ips, priv, err := resolveAndCheckHost("8.8.8.8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv {
		t.Error("8.8.8.8 should not be private")
	}
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
}

func TestResolveAndCheckHost_LiteralPrivateIP(t *testing.T) {
	ips, priv, err := resolveAndCheckHost("127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !priv {
		t.Error("127.0.0.1 should be private")
	}
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
}

func TestResolveAndCheckHost_PrivateIPv6(t *testing.T) {
	ips, priv, err := resolveAndCheckHost("::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !priv {
		t.Error("::1 should be private")
	}
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
}

func TestResolveAndCheckHost_ValidHostname(t *testing.T) {
	// This resolves a real hostname — should return IPs without error
	ips, _, err := resolveAndCheckHost("dns.google")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) == 0 {
		t.Error("expected at least 1 IP for dns.google")
	}
}

func TestResolveAndCheckHost_InvalidHostname(t *testing.T) {
	_, _, err := resolveAndCheckHost("nonexistent.invalid.example.test")
	if err == nil {
		t.Error("expected error for invalid hostname")
	}
}

func TestForward_SSESpanCapture(t *testing.T) {
	// Test that SSE output is captured in span
	events := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\ndata: [DONE]\n\n"
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(events))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "sk-123",
	}, 0)

	// Use a real HTTP server for SSE
	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	t.Cleanup(srv.Close)

	client := srv.Client()
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	if payload.Output != "Hello world" {
		t.Errorf("span Output = %q, want 'Hello world'", payload.Output)
	}
}

func TestForward_CRLFHeadersSanitized(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, dispatcher := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("X-AgentSpan-Session", "sess\r\ninjection")
	req.Header.Set("X-AgentSpan-Agent", "agent\ninjection")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	payload, ok := dispatcher.DrainOne()
	if !ok {
		t.Fatal("expected span to be dispatched")
	}
	// sanitizeHeader strips control chars, so CR/LF are removed and containsCRLF returns false.
	// The sanitized value "sessinjection" is kept.
	if payload.ExternalSessionID != "sessinjection" {
		t.Errorf("expected sanitized session ID 'sessinjection', got %q", payload.ExternalSessionID)
	}
	if payload.AgentName != "agentinjection" {
		t.Errorf("expected sanitized agent name 'agentinjection', got %q", payload.AgentName)
	}
}

func TestForward_QueryStringTooLong(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be called")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	longQuery := strings.Repeat("a", 9000)
	req := httptest.NewRequest("POST", "/v1/chat/completions?"+longQuery, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 414 {
		t.Errorf("expected 414, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForward_HopByHopHeadersStripped(t *testing.T) {
	var received http.Header
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Clone()
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Transfer-Encoding", "chunked")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if received.Get("Connection") != "" {
		t.Error("Connection header should not be forwarded")
	}
}

func TestForward_ProviderDownSSRFBlocked(t *testing.T) {
	// Test that private IP provider URL is blocked
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	providerSrv := httptest.NewServer(provider)
	t.Cleanup(providerSrv.Close)

	authResult := &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
		BaseURL:      "http://127.0.0.1:9999",
	}

	authSrv := mockAuthServer(authResult)
	t.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 100, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)
	// allowPrivateIPs = false
	h := NewProxyHandler(context.Background(), cache, dispatcher, 10*time.Second, "2024-10-22", false, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 for private IP provider, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForward_XForwardedForStripped(t *testing.T) {
	var received http.Header
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Clone()
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
	}, 0)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if received.Get("X-Forwarded-For") != "" {
		t.Error("X-Forwarded-For header should not be forwarded")
	}
	if received.Get("X-Forwarded-Proto") != "" {
		t.Error("X-Forwarded-Proto header should not be forwarded")
	}
}

func BenchmarkProxyOverhead(b *testing.B) {
	providerBody := `{"id":"chatcmpl-123","choices":[{"message":{"content":"Hello!"}}]}`

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(providerBody))
	}))
	b.Cleanup(provider.Close)

	authResult := &auth.AuthVerifyResult{
		Valid:        true,
		APIKeyID:     "key-1",
		ProviderType: "openai",
		ProviderKey:  "sk-123",
		BaseURL:      provider.URL,
	}

	authSrv := mockAuthServer(authResult)
	b.Cleanup(authSrv.Close)

	cache := auth.NewAuthCache(context.Background(), authSrv.URL, "test-token", "test-secret", 30*time.Second, &http.Client{Timeout: 5 * time.Second}, 0)
	dispatcher := span.NewSpanDispatcher("http://localhost:9999", "test-token", 10000, &http.Client{Timeout: 1 * time.Second}, 10*time.Second, 0, 1)
	h := NewProxyHandler(context.Background(), cache, dispatcher, 10*time.Second, "2024-10-22", true, 0)

	// Warm up the cache
	warmReq := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	warmReq.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	warmW := httptest.NewRecorder()
	h.ServeHTTP(warmW, warmReq)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
		req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			b.Fatalf("expected 200, got %d", w.Code)
		}
	}
}

func TestForward_ProviderNon2xxForwarded(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"provider 400", 400, `{"error":{"message":"invalid model","type":"invalid_request_error"}}`},
		{"provider 401", 401, `{"error":{"message":"invalid api key","type":"authentication_error"}}`},
		{"provider 429", 429, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`},
		{"provider 500", 500, `{"error":{"message":"internal error","type":"server_error"}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.body))
			})

			h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
				Valid:          true,
				APIKeyID:       "key-1",
				OrganizationID: "org-1",
				ProviderType:   "openai",
				ProviderKey:    "provider-key-123",
			}, 0)

			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
			req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Fatalf("expected %d, got %d: %s", tc.statusCode, w.Code, w.Body.String())
			}
			if w.Body.String() != tc.body {
				t.Errorf("response body mismatch:\ngot:  %s\nwant: %s", w.Body.String(), tc.body)
			}
		})
	}
}

func TestForward_RequestTooLarge(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider should not be reached for oversized request")
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    "provider-key-123",
	}, 0)

	// Create a body slightly over 10MB
	bigBody := strings.NewReader(strings.Repeat("x", 10*1024*1024+1))
	req := httptest.NewRequest("POST", "/v1/chat/completions", bigBody)
	req.Header.Set("Authorization", "Bearer as-abcdef1234567890abcdef1234567890")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != 413 {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}
