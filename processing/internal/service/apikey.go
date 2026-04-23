package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/crypto"
	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/google/uuid"
)

// validProviderTypes is the set of accepted provider_type values.
var validProviderTypes = map[string]bool{
	"openai":    true,
	"anthropic": true,
	"deepseek":  true,
	"mistral":   true,
	"groq":      true,
	"gemini":    true,
	"custom":    true,
}

// defaultTestModels maps known provider types to cheap models for test requests.
var defaultTestModels = map[string]string{
	"openai":    "gpt-4o-mini",
	"anthropic": "claude-haiku-4-5-20251001",
	"deepseek":  "deepseek-chat",
	"mistral":   "mistral-small-latest",
	"groq":      "llama-3.1-8b-instant",
	"gemini":    "gemini-2.0-flash-lite",
}

// APIKeyService handles API key business logic: creation, listing, and deactivation.
type APIKeyService struct {
	queries         *db.Queries
	hmacSecret      string
	encryptionKey   string
	internalService *InternalService
	httpClient      *http.Client
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(queries *db.Queries, hmacSecret, encryptionKey string) *APIKeyService {
	return &APIKeyService{
		queries:       queries,
		hmacSecret:    hmacSecret,
		encryptionKey: encryptionKey,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

// SetInternalService injects InternalService for span ingestion during key testing.
func (s *APIKeyService) SetInternalService(is *InternalService) {
	s.internalService = is
}

// APIKeyCreateResult is returned from CreateAPIKey. RawKey is the plaintext AgentOrbit API key
// shown once at creation — it is never retrievable again.
type APIKeyCreateResult struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"`
	BaseURL      *string   `json:"base_url,omitempty"`
	Display      string    `json:"display"`
	RawKey       string    `json:"raw_key"` // shown once, never again
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIKeyListItem is used for list and get responses. It contains no sensitive data.
type APIKeyListItem struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	ProviderType string     `json:"provider_type"`
	BaseURL      *string    `json:"base_url,omitempty"`
	Display      string     `json:"display"`
	Active       bool       `json:"active"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CreateAPIKey generates a new AgentOrbit API key, encrypts the provider key at rest,
// and stores HMAC-SHA256 digest of the raw key (SEC-01, SEC-02, AKEY-01, AKEY-02, AUTH-05).
// The raw AgentOrbit API key is returned once and never stored or retrievable again (AUTH-06).
func (s *APIKeyService) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name, providerType, providerKey string, baseURL *string) (*APIKeyCreateResult, error) {
	// Validate name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Code: "name_required", Message: "Name is required"}
	}
	if len(name) > 100 {
		return nil, &ValidationError{Code: "name_too_long", Message: "Name must be 100 characters or fewer"}
	}

	// Validate provider type
	if !validProviderTypes[providerType] {
		return nil, &ValidationError{Code: "invalid_provider_type", Message: "provider_type must be one of: openai, anthropic, deepseek, mistral, groq, gemini, custom"}
	}

	// Custom provider requires base_url
	if providerType == "custom" && (baseURL == nil || strings.TrimSpace(*baseURL) == "") {
		return nil, &ValidationError{Code: "base_url_required", Message: "base_url is required for custom provider type"}
	}

	// Normalize base URL: strip trailing slashes to prevent double-slash in constructed URLs.
	if baseURL != nil && *baseURL != "" {
		trimmed := strings.TrimRight(*baseURL, "/")
		baseURL = &trimmed
	}

	// Validate provider key
	if strings.TrimSpace(providerKey) == "" {
		return nil, &ValidationError{Code: "provider_key_required", Message: "Provider key is required"}
	}

	// Generate AgentOrbit API key: raw = "ao-" + hex(16 random bytes)
	raw, digest, display, err := crypto.GenerateAPIKey(s.hmacSecret)
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	// Encrypt provider key at rest with AES-256-GCM
	encrypted, err := crypto.Encrypt([]byte(providerKey), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt provider key: %w", err)
	}

	// Store — only digest is persisted for the AgentOrbit key, raw is never stored
	row, err := s.queries.CreateApiKey(ctx, db.CreateApiKeyParams{
		OrganizationID:       orgID,
		Name:                 name,
		ProviderType:         providerType,
		ProviderKeyEncrypted: encrypted,
		BaseUrl:              baseURL,
		KeyDigest:            digest,
		Display:              display,
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &APIKeyCreateResult{
		ID:           row.ID,
		Name:         row.Name,
		ProviderType: row.ProviderType,
		BaseURL:      row.BaseUrl,
		Display:      row.Display,
		RawKey:       raw,
		Active:       row.Active,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// ListAPIKeys returns all API keys for an organization with masked display format (AKEY-04).
// Provider keys and key digests are never included in the response.
func (s *APIKeyService) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]APIKeyListItem, error) {
	rows, err := s.queries.ListApiKeysByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	items := make([]APIKeyListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toListItem(row))
	}
	return items, nil
}

// DeactivateAPIKey deactivates an API key — this is irreversible (AKEY-03).
func (s *APIKeyService) DeactivateAPIKey(ctx context.Context, orgID, keyID uuid.UUID) error {
	err := s.queries.DeactivateApiKey(ctx, db.DeactivateApiKeyParams{
		ID:             keyID,
		OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("deactivate api key: %w", err)
	}
	return nil
}

// GetAPIKey returns a single API key by ID, scoped to the organization.
// No sensitive fields are returned.
func (s *APIKeyService) GetAPIKey(ctx context.Context, orgID, keyID uuid.UUID) (*APIKeyListItem, error) {
	row, err := s.queries.GetApiKeyByID(ctx, db.GetApiKeyByIDParams{
		ID:             keyID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	item := APIKeyListItem{
		ID:           row.ID,
		Name:         row.Name,
		ProviderType: row.ProviderType,
		BaseURL:      row.BaseUrl,
		Display:      row.Display,
		Active:       row.Active,
		CreatedAt:    row.CreatedAt,
	}
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		item.LastUsedAt = &t
	}
	return &item, nil
}

// toListItem maps a db.ListApiKeysByOrgRow to APIKeyListItem, excluding all sensitive fields.
func toListItem(row db.ListApiKeysByOrgRow) APIKeyListItem {
	item := APIKeyListItem{
		ID:           row.ID,
		Name:         row.Name,
		ProviderType: row.ProviderType,
		BaseURL:      row.BaseUrl,
		Display:      row.Display,
		Active:       row.Active,
		CreatedAt:    row.CreatedAt,
	}
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		item.LastUsedAt = &t
	}
	return item
}

// TestKeyRequest is the optional body for POST /api/orgs/{orgID}/keys/{keyID}/test.
type TestKeyRequest struct {
	Model string `json:"model,omitempty"`
}

// TestKeyResult is the response from a successful key test.
type TestKeyResult struct {
	Success  bool   `json:"success"`
	Model    string `json:"model"`
	Response string `json:"response"`
}

// TestAPIKey sends a real request to the provider to verify the key works, then records
// the span via IngestSpan so it appears in the dashboard with system prompt extraction.
// For known providers, a cheap default model is used. For custom/unknown providers, model is required.
func (s *APIKeyService) TestAPIKey(ctx context.Context, orgID, keyID uuid.UUID, model string) (*TestKeyResult, error) {
	// Fetch the key (includes encrypted provider key).
	apiKey, err := s.queries.GetApiKeyByID(ctx, db.GetApiKeyByIDParams{
		ID:             keyID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, &ServiceError{Status: 404, Code: "not_found", Message: "API key not found"}
	}
	if !apiKey.Active {
		return nil, &ServiceError{Status: 409, Code: "key_inactive", Message: "API key is deactivated"}
	}

	// Decrypt provider key.
	providerKeyBytes, err := crypto.Decrypt(apiKey.ProviderKeyEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("test key: decrypt provider key: %w", err)
	}
	providerKey := string(providerKeyBytes)

	// Resolve model.
	if model == "" {
		m, ok := defaultTestModels[apiKey.ProviderType]
		if !ok {
			return nil, &ServiceError{Status: 422, Code: "model_required", Message: "Model is required for this provider type"}
		}
		model = m
	}

	// Resolve base URL.
	baseURL := ""
	if apiKey.BaseUrl != nil {
		baseURL = *apiKey.BaseUrl
	}
	if baseURL == "" {
		if u, ok := defaultBaseURLs[apiKey.ProviderType]; ok {
			baseURL = u
		} else {
			return nil, &ServiceError{Status: 422, Code: "base_url_required", Message: "Base URL is required for this provider type"}
		}
	}

	systemPrompt := "You are AgentOrbit test assistant. Confirm the connection is working in one sentence."
	userMessage := "Hello, is everything connected?"

	start := time.Now()
	var result *TestKeyResult
	var httpStatus int32

	if apiKey.ProviderType == "anthropic" {
		result, httpStatus, err = s.testAnthropic(ctx, baseURL, providerKey, model, systemPrompt, userMessage)
	} else {
		result, httpStatus, err = s.testOpenAICompat(ctx, baseURL, providerKey, model, systemPrompt, userMessage)
	}
	durationMs := int32(time.Since(start).Milliseconds())

	// Record the span via IngestSpan (best-effort — don't fail the test if ingestion fails).
	// Use the same "role: content\n" text format that the proxy's extractInputText produces.
	if s.internalService != nil {
		var input, output string
		input = "system: " + systemPrompt + "\nuser: " + userMessage + "\n"
		if result != nil {
			output = result.Response
		}
		if err != nil {
			httpStatus = 500
		}

		spanReq := &SpanIngestRequest{
			APIKeyID:       keyID.String(),
			OrganizationID: orgID.String(),
			ProviderType:   apiKey.ProviderType,
			Model:          model,
			Input:          input,
			Output:         output,
			InputTokens:    0,
			OutputTokens:   0,
			DurationMs:     durationMs,
			HTTPStatus:     httpStatus,
			StartedAt:      start.Format(time.RFC3339Nano),
			FinishReason:   "stop",
			AgentName:      "key-test",
		}
		// Fire and forget — test result is what matters.
		_ = s.internalService.IngestSpan(ctx, spanReq)
	}

	return result, err
}

// testOpenAICompat sends a test request directly to the provider using OpenAI chat completions format.
func (s *APIKeyService) testOpenAICompat(ctx context.Context, baseURL, providerKey, model, systemPrompt, userMessage string) (*TestKeyResult, int32, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
		"max_tokens": 100,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("test key: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+providerKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, &ServiceError{Status: 502, Code: "provider_unreachable", Message: "Failed to reach provider: " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != 200 {
		return nil, int32(resp.StatusCode), &ServiceError{
			Status:  502,
			Code:    "provider_error",
			Message: fmt.Sprintf("Provider returned %d: %s", resp.StatusCode, truncateStr(string(respBody), 200)),
		}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Choices) == 0 {
		return nil, int32(resp.StatusCode), &ServiceError{Status: 502, Code: "invalid_response", Message: "Unexpected response format from provider"}
	}

	return &TestKeyResult{
		Success:  true,
		Model:    model,
		Response: result.Choices[0].Message.Content,
	}, int32(resp.StatusCode), nil
}

// testAnthropic sends a test request directly to the provider using Anthropic messages format.
func (s *APIKeyService) testAnthropic(ctx context.Context, baseURL, providerKey, model, systemPrompt, userMessage string) (*TestKeyResult, int32, error) {
	body := map[string]any{
		"model":  model,
		"system": systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
		"max_tokens": 100,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("test key: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", providerKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, &ServiceError{Status: 502, Code: "provider_unreachable", Message: "Failed to reach provider: " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != 200 {
		return nil, int32(resp.StatusCode), &ServiceError{
			Status:  502,
			Code:    "provider_error",
			Message: fmt.Sprintf("Provider returned %d: %s", resp.StatusCode, truncateStr(string(respBody), 200)),
		}
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Content) == 0 {
		return nil, int32(resp.StatusCode), &ServiceError{Status: 502, Code: "invalid_response", Message: "Unexpected response format from provider"}
	}

	return &TestKeyResult{
		Success:  true,
		Model:    model,
		Response: result.Content[0].Text,
	}, int32(resp.StatusCode), nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ValidationError represents an input validation failure.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
