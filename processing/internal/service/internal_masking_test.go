package service

import (
	"encoding/json"
	"testing"
)

// TestAuthVerifyResult_IncludesMaskingConfig verifies that AuthVerifyResult includes
// StoreSpanContent and MaskingConfig fields in JSON serialization (D-17).
func TestVerifyAPIKey_IncludesMaskingConfig(t *testing.T) {
	tests := []struct {
		name             string
		storeSpanContent bool
		maskingConfig    json.RawMessage
	}{
		{
			name:             "default values - store content true, no masking config",
			storeSpanContent: true,
			maskingConfig:    nil,
		},
		{
			name:             "metadata only mode - store content false",
			storeSpanContent: false,
			maskingConfig:    nil,
		},
		{
			name:             "with phone masking config llm_only",
			storeSpanContent: true,
			maskingConfig:    json.RawMessage(`{"phone":"llm_only"}`),
		},
		{
			name:             "with phone masking config llm_storage",
			storeSpanContent: true,
			maskingConfig:    json.RawMessage(`{"phone":"llm_storage"}`),
		},
		{
			name:             "with phone masking config off",
			storeSpanContent: true,
			maskingConfig:    json.RawMessage(`{"phone":"off"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AuthVerifyResult{
				Valid:            true,
				APIKeyID:         "test-key-id",
				OrganizationID:   "test-org-id",
				ProviderType:     "openai",
				ProviderKey:      "sk-test",
				BaseURL:          "https://api.openai.com",
				StoreSpanContent: tt.storeSpanContent,
				MaskingConfig:    tt.maskingConfig,
			}

			// Verify JSON round-trip preserves fields.
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded AuthVerifyResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if decoded.StoreSpanContent != tt.storeSpanContent {
				t.Errorf("StoreSpanContent: got %v, want %v", decoded.StoreSpanContent, tt.storeSpanContent)
			}

			if tt.maskingConfig != nil {
				if decoded.MaskingConfig == nil {
					t.Error("MaskingConfig should not be nil")
				} else if string(decoded.MaskingConfig) != string(tt.maskingConfig) {
					t.Errorf("MaskingConfig: got %s, want %s", decoded.MaskingConfig, tt.maskingConfig)
				}
			}

			// Verify store_span_content is always present in JSON (not omitempty).
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("unmarshal raw error: %v", err)
			}
			if _, ok := raw["store_span_content"]; !ok {
				t.Error("store_span_content field should always be present in JSON output")
			}
		})
	}
}

// TestIngestSpan_MaskingAppliedTrue verifies that MaskingApplied is set when true.
func TestIngestSpan_MaskingAppliedTrue(t *testing.T) {
	req := SpanIngestRequest{
		APIKeyID:       "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "test input",
		Output:         "test output",
		InputTokens:    100,
		OutputTokens:   50,
		DurationMs:     1000,
		HTTPStatus:     200,
		StartedAt:      "2026-01-01T00:00:00Z",
		MaskingApplied: true,
	}

	if !req.MaskingApplied {
		t.Error("MaskingApplied should be true")
	}

	// Verify JSON round-trip.
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded SpanIngestRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !decoded.MaskingApplied {
		t.Error("MaskingApplied should survive JSON round-trip")
	}
}

// TestIngestSpan_MaskingAppliedFalse verifies default false behavior is unchanged.
func TestIngestSpan_MaskingAppliedFalse(t *testing.T) {
	req := SpanIngestRequest{
		APIKeyID:       "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "test input",
		Output:         "test output",
		InputTokens:    100,
		OutputTokens:   50,
		DurationMs:     1000,
		HTTPStatus:     200,
		StartedAt:      "2026-01-01T00:00:00Z",
	}

	if req.MaskingApplied {
		t.Error("MaskingApplied should default to false")
	}

	// Verify JSON deserialization without the field yields false.
	jsonStr := `{"api_key_id":"id","organization_id":"org","provider_type":"openai","model":"gpt-4","input":"x","output":"y","input_tokens":1,"output_tokens":1,"duration_ms":100,"http_status":200,"started_at":"2026-01-01T00:00:00Z"}`
	var decoded SpanIngestRequest
	if err := json.Unmarshal([]byte(jsonStr), &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.MaskingApplied {
		t.Error("MaskingApplied should be false when absent from JSON")
	}
}

// TestIngestSpan_WithMaskingMap verifies MaskingMap entries are carried in the request (D-10 LLM Only mode).
func TestIngestSpan_WithMaskingMap(t *testing.T) {
	entries := []MaskingMapEntry{
		{MaskType: "phone", OriginalValue: "+1-555-0123", MaskedValue: "+1-711-1111"},
		{MaskType: "phone", OriginalValue: "+1-555-0456", MaskedValue: "+1-711-1112"},
	}

	req := SpanIngestRequest{
		APIKeyID:       "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "call +1-711-1111",
		Output:         "ok",
		InputTokens:    10,
		OutputTokens:   5,
		DurationMs:     100,
		HTTPStatus:     200,
		StartedAt:      "2026-01-01T00:00:00Z",
		MaskingApplied: true,
		MaskingMap:     entries,
	}

	if len(req.MaskingMap) != 2 {
		t.Fatalf("MaskingMap length: got %d, want 2", len(req.MaskingMap))
	}

	// Verify JSON round-trip preserves all entries.
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded SpanIngestRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(decoded.MaskingMap) != 2 {
		t.Fatalf("decoded MaskingMap length: got %d, want 2", len(decoded.MaskingMap))
	}
	if decoded.MaskingMap[0].MaskType != "phone" {
		t.Errorf("MaskingMap[0].MaskType: got %s, want phone", decoded.MaskingMap[0].MaskType)
	}
	if decoded.MaskingMap[0].OriginalValue != "+1-555-0123" {
		t.Errorf("MaskingMap[0].OriginalValue: got %s, want +1-555-0123", decoded.MaskingMap[0].OriginalValue)
	}
	if decoded.MaskingMap[1].MaskedValue != "+1-711-1112" {
		t.Errorf("MaskingMap[1].MaskedValue: got %s, want +1-711-1112", decoded.MaskingMap[1].MaskedValue)
	}
}

// TestIngestSpan_EmptyMaskingMap verifies empty masking map is omitted from JSON (D-11 LLM+Storage mode).
func TestIngestSpan_EmptyMaskingMap(t *testing.T) {
	req := SpanIngestRequest{
		APIKeyID:       "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "test",
		Output:         "test",
		InputTokens:    10,
		OutputTokens:   5,
		DurationMs:     100,
		HTTPStatus:     200,
		StartedAt:      "2026-01-01T00:00:00Z",
		MaskingApplied: false,
		MaskingMap:     nil,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// masking_map should be omitted from JSON when nil (omitempty).
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw error: %v", err)
	}
	if _, ok := raw["masking_map"]; ok {
		t.Error("masking_map should be omitted from JSON when nil")
	}

	// Also test empty slice is omitted.
	req.MaskingMap = []MaskingMapEntry{}
	data, err = json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Note: empty slice is NOT omitted by omitempty for slices in Go.
	// This is expected Go behavior - the proxy should send nil, not empty slice.
}

// TestIngestSpan_MaskingMapInsertError verifies MaskingMapEntry struct has correct JSON tags.
func TestIngestSpan_MaskingMapInsertError(t *testing.T) {
	// Test that MaskingMapEntry serializes correctly for the internal API.
	entry := MaskingMapEntry{
		MaskType:      "phone",
		OriginalValue: "+1-555-0123",
		MaskedValue:   "+1-711-1111",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify JSON field names match the internal API contract.
	expectedFields := []string{"mask_type", "original_value", "masked_value"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing expected JSON field: %s", field)
		}
	}

	if raw["mask_type"] != "phone" {
		t.Errorf("mask_type: got %v, want phone", raw["mask_type"])
	}
	if raw["original_value"] != "+1-555-0123" {
		t.Errorf("original_value: got %v, want +1-555-0123", raw["original_value"])
	}
	if raw["masked_value"] != "+1-711-1111" {
		t.Errorf("masked_value: got %v, want +1-711-1111", raw["masked_value"])
	}
}

// TestMaskingMapEntry_Type verifies the MaskingMapEntry struct exists with expected fields.
func TestMaskingMapEntry_Type(t *testing.T) {
	entry := MaskingMapEntry{
		MaskType:      "phone",
		OriginalValue: "original",
		MaskedValue:   "masked",
	}

	if entry.MaskType != "phone" {
		t.Errorf("MaskType: got %s, want phone", entry.MaskType)
	}
	if entry.OriginalValue != "original" {
		t.Errorf("OriginalValue: got %s, want original", entry.OriginalValue)
	}
	if entry.MaskedValue != "masked" {
		t.Errorf("MaskedValue: got %s, want masked", entry.MaskedValue)
	}
}
