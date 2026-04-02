package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentspan/processing/internal/crypto"
	"github.com/agentspan/processing/internal/db"
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

// APIKeyService handles API key business logic: creation, listing, and deactivation.
type APIKeyService struct {
	queries       *db.Queries
	hmacSecret    string
	encryptionKey string
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(queries *db.Queries, hmacSecret, encryptionKey string) *APIKeyService {
	return &APIKeyService{
		queries:       queries,
		hmacSecret:    hmacSecret,
		encryptionKey: encryptionKey,
	}
}

// APIKeyCreateResult is returned from CreateAPIKey. RawKey is the plaintext AgentSpan API key
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

// CreateAPIKey generates a new AgentSpan API key, encrypts the provider key at rest,
// and stores HMAC-SHA256 digest of the raw key (SEC-01, SEC-02, AKEY-01, AKEY-02, AUTH-05).
// The raw AgentSpan API key is returned once and never stored or retrievable again (AUTH-06).
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

	// Validate provider key
	if strings.TrimSpace(providerKey) == "" {
		return nil, &ValidationError{Code: "provider_key_required", Message: "Provider key is required"}
	}

	// Generate AgentSpan API key: raw = "as-" + hex(16 random bytes)
	raw, digest, display, err := crypto.GenerateAPIKey(s.hmacSecret)
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	// Encrypt provider key at rest with AES-256-GCM
	encrypted, err := crypto.Encrypt([]byte(providerKey), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt provider key: %w", err)
	}

	// Store — only digest is persisted for the AgentSpan key, raw is never stored
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

// ValidationError represents an input validation failure.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
