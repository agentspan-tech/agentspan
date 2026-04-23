package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxLLMResponseSize = 1 << 20 // 1MB limit for LLM response bodies

// truncateForError returns a truncated string suitable for error messages (max 200 bytes).
func truncateForError(b []byte) string {
	if len(b) <= 200 {
		return string(b)
	}
	return string(b[:200]) + "...(truncated)"
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client is the interface for LLM completions.
type Client interface {
	Complete(ctx context.Context, messages []Message) (string, error)
}

// NewClient returns a Client based on base URL, API key, and model.
// Returns nil if any of the three is empty.
// If the base URL contains "anthropic.com", an Anthropic-native client is used;
// otherwise an OpenAI-compatible client is used (works with DeepSeek, OpenRouter, etc.).
func NewClient(apiKey, model, baseURL string) Client {
	if baseURL == "" || apiKey == "" || model == "" {
		return nil
	}
	if strings.Contains(baseURL, "anthropic.com") {
		return &anthropicClient{
			apiKey:     apiKey,
			model:      model,
			baseURL:    baseURL,
			httpClient: &http.Client{Timeout: 60 * time.Second},
		}
	}
	return &openAIClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// openAIClient calls OpenAI-compatible chat completions endpoints.
type openAIClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func (c *openAIClient) Complete(ctx context.Context, messages []Message) (string, error) {
	body := map[string]any{
		"model":      c.model,
		"messages":   messages,
		"max_tokens": 1024,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm openai: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llm openai: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm openai: do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseSize))
	if err != nil {
		return "", fmt.Errorf("llm openai: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm openai: status %d: %s", resp.StatusCode, truncateForError(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("llm openai: parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llm openai: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// anthropicClient calls Anthropic-compatible messages endpoints.
type anthropicClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func (c *anthropicClient) Complete(ctx context.Context, messages []Message) (string, error) {
	// Extract system message if present, remaining messages go into messages array.
	var systemPrompt string
	var userMessages []Message
	for _, m := range messages {
		if m.Role == "system" {
			if systemPrompt == "" {
				systemPrompt = m.Content
			}
		} else {
			userMessages = append(userMessages, m)
		}
	}

	body := map[string]any{
		"model":      c.model,
		"messages":   userMessages,
		"max_tokens": 1024,
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm anthropic: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("llm anthropic: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm anthropic: do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseSize))
	if err != nil {
		return "", fmt.Errorf("llm anthropic: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm anthropic: status %d: %s", resp.StatusCode, truncateForError(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("llm anthropic: parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("llm anthropic: no content in response")
	}
	return result.Content[0].Text, nil
}
