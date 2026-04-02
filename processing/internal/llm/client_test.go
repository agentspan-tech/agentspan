package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_OpenAI(t *testing.T) {
	c := NewClient("openai", "sk-test", "gpt-4", "")
	if c == nil {
		t.Fatal("expected non-nil client for openai")
	}
}

func TestNewClient_Anthropic(t *testing.T) {
	c := NewClient("anthropic", "sk-ant-test", "claude-3", "")
	if c == nil {
		t.Fatal("expected non-nil client for anthropic")
	}
}

func TestNewClient_EmptyProvider(t *testing.T) {
	c := NewClient("", "key", "model", "")
	if c != nil {
		t.Error("expected nil client for empty provider")
	}
}

func TestNewClient_EmptyModel(t *testing.T) {
	c := NewClient("openai", "key", "", "")
	if c != nil {
		t.Error("expected nil client for empty model")
	}
}

func TestNewClient_UnknownProvider(t *testing.T) {
	c := NewClient("unknown", "key", "model", "")
	if c != nil {
		t.Error("expected nil client for unknown provider")
	}
}

func TestOpenAIClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
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

	c := NewClient("openai", "test-key", "gpt-4", srv.URL)
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

	c := NewClient("openai", "test-key", "gpt-4", srv.URL)
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

	c := NewClient("openai", "test-key", "gpt-4", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestAnthropicClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
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

	c := NewClient("anthropic", "test-key", "claude-3", srv.URL)
	result, err := c.Complete(context.Background(), []Message{
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

	c := NewClient("anthropic", "test-key", "claude-3", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}

func TestAnthropicClient_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
	}))
	defer srv.Close()

	c := NewClient("anthropic", "test-key", "claude-3", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
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

	c := NewClient("anthropic", "test-key", "claude-3", srv.URL)
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "Hi"}})
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

func TestNewClient_CustomBaseURL(t *testing.T) {
	c := NewClient("openai", "key", "model", "http://custom.example.com")
	oc, ok := c.(*openAIClient)
	if !ok {
		t.Fatal("expected openAIClient")
	}
	if oc.baseURL != "http://custom.example.com" {
		t.Errorf("baseURL = %q", oc.baseURL)
	}
}

func TestNewClient_DefaultBaseURLs(t *testing.T) {
	oc := NewClient("openai", "key", "model", "").(*openAIClient)
	if oc.baseURL != "https://api.openai.com" {
		t.Errorf("openai default baseURL = %q", oc.baseURL)
	}

	ac := NewClient("anthropic", "key", "model", "").(*anthropicClient)
	if ac.baseURL != "https://api.anthropic.com" {
		t.Errorf("anthropic default baseURL = %q", ac.baseURL)
	}
}
