package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_OpenAICompatible(t *testing.T) {
	c := NewClient("sk-test", "gpt-4", "https://api.openai.com")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := c.(*openAIClient); !ok {
		t.Fatal("expected openAIClient for non-anthropic URL")
	}
}

func TestNewClient_AnthropicAutoDetect(t *testing.T) {
	c := NewClient("sk-ant-test", "claude-3", "https://api.anthropic.com")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := c.(*anthropicClient); !ok {
		t.Fatal("expected anthropicClient for anthropic.com URL")
	}
}

func TestNewClient_DeepSeek(t *testing.T) {
	c := NewClient("sk-test", "deepseek-chat", "https://api.deepseek.com")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := c.(*openAIClient); !ok {
		t.Fatal("expected openAIClient for DeepSeek")
	}
}

func TestNewClient_EmptyBaseURL(t *testing.T) {
	c := NewClient("key", "model", "")
	if c != nil {
		t.Error("expected nil client for empty base URL")
	}
}

func TestNewClient_EmptyAPIKey(t *testing.T) {
	c := NewClient("", "model", "https://api.openai.com")
	if c != nil {
		t.Error("expected nil client for empty API key")
	}
}

func TestNewClient_EmptyModel(t *testing.T) {
	c := NewClient("key", "", "https://api.openai.com")
	if c != nil {
		t.Error("expected nil client for empty model")
	}
}

func TestOpenAIClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Hello from OpenAI"}},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-key", "gpt-4", srv.URL)
	result, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello from OpenAI" {
		t.Errorf("result = %q, want 'Hello from OpenAI'", result)
	}
}

func TestOpenAIClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	c := NewClient("test-key", "gpt-4", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestOpenAIClient_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	c := NewClient("test-key", "gpt-4", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestAnthropicClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path = %q, want /messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}

		// Verify system prompt is extracted
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["system"] != "You are helpful" {
			t.Errorf("system = %v", body["system"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"text": "Hello from Anthropic"},
			},
		})
	}))
	defer srv.Close()

	// Use a URL containing "anthropic.com" to trigger Anthropic client auto-detection.
	// httptest URLs are localhost, so we test the anthropicClient directly.
	ac := &anthropicClient{
		apiKey:     "test-key",
		model:      "claude-3",
		baseURL:    srv.URL,
		httpClient: http.DefaultClient,
	}
	result, err := ac.Complete(context.Background(), []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello from Anthropic" {
		t.Errorf("result = %q, want 'Hello from Anthropic'", result)
	}
}

func TestAnthropicClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	ac := &anthropicClient{
		apiKey:     "test-key",
		model:      "claude-3",
		baseURL:    srv.URL,
		httpClient: http.DefaultClient,
	}
	_, err := ac.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}

func TestAnthropicClient_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
	}))
	defer srv.Close()

	ac := &anthropicClient{
		apiKey:     "test-key",
		model:      "claude-3",
		baseURL:    srv.URL,
		httpClient: http.DefaultClient,
	}
	_, err := ac.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestAnthropicClient_NoSystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["system"]; ok {
			t.Error("system should not be set when no system message")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"text": "OK"}},
		})
	}))
	defer srv.Close()

	ac := &anthropicClient{
		apiKey:     "test-key",
		model:      "claude-3",
		baseURL:    srv.URL,
		httpClient: http.DefaultClient,
	}
	_, err := ac.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTruncateForError(t *testing.T) {
	short := "short message"
	if truncateForError([]byte(short)) != short {
		t.Error("short message should not be truncated")
	}

	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	result := truncateForError(long)
	if len(result) > 220 {
		t.Errorf("truncated result too long: %d", len(result))
	}
}
