package masking

import (
	"strings"
	"testing"
)

func TestRegexMask_PhonePreset(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMOnly,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	cfg.Compile()
	content := []byte("Call me at +79509191919 please")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Original != "+79509191919" {
		t.Errorf("expected original +79509191919, got %s", result.Entries[0].Original)
	}
	if !strings.Contains(result.Entries[0].Masked, "[PHONE_") {
		t.Errorf("expected masked to contain [PHONE_, got %s", result.Entries[0].Masked)
	}
	if strings.Contains(string(result.Content), "+79509191919") {
		t.Error("original phone number still present in result")
	}
}

func TestRegexMask_MultiplePhones(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMOnly,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	cfg.Compile()
	content := []byte("A: +79509191919, B: +79509884417, C: +79001112233")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}
	resultStr := string(result.Content)
	if strings.Contains(resultStr, "+7950") || strings.Contains(resultStr, "+7900") {
		t.Error("original phone numbers still present in result")
	}
}

func TestRegexMask_DuplicatePhones(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMOnly,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	cfg.Compile()
	content := []byte("Call +79509191919 or +79509191919")
	result := ApplyMasking(content, &cfg)
	// Same number should produce only 1 entry
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (deduped), got %d", len(result.Entries))
	}
}

func TestRegexMask_CustomRule(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMOnly,
		Rules: []MaskingRule{
			{Name: "email", Pattern: `\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`, Builtin: false},
		},
	}
	cfg.Compile()
	content := []byte("Contact user@example.com for help")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Original != "user@example.com" {
		t.Errorf("expected original user@example.com, got %s", result.Entries[0].Original)
	}
	if !strings.Contains(result.Entries[0].Masked, "[EMAIL_") {
		t.Errorf("expected masked to contain [EMAIL_, got %s", result.Entries[0].Masked)
	}
}

func TestRegexMask_MultipleRules(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMStorage,
		Rules: []MaskingRule{
			PresetPhoneRule,
			{Name: "ssn", Pattern: `\d{3}-\d{2}-\d{4}`, Builtin: false},
		},
	}
	cfg.Compile()
	content := []byte("Phone: +79509191919, SSN: 123-45-6789")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	resultStr := string(result.Content)
	if strings.Contains(resultStr, "+79509191919") || strings.Contains(resultStr, "123-45-6789") {
		t.Error("original values still present in result")
	}
	// Each rule type should have its own counter starting at 1.
	if result.Entries[0].Masked != "[PHONE_1]" {
		t.Errorf("phone placeholder should be [PHONE_1], got %s", result.Entries[0].Masked)
	}
	if result.Entries[1].Masked != "[SSN_1]" {
		t.Errorf("ssn placeholder should be [SSN_1], got %s", result.Entries[1].Masked)
	}
}

func TestRegexMask_ModeOff(t *testing.T) {
	cfg := MaskingConfig{
		Mode:  MaskModeOff,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	content := []byte("Call +79509191919")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for off mode, got %d", len(result.Entries))
	}
	if string(result.Content) != string(content) {
		t.Error("content should be unchanged for off mode")
	}
}

func TestRegexMask_EmptyRules(t *testing.T) {
	cfg := MaskingConfig{
		Mode:  MaskModeLLMOnly,
		Rules: nil,
	}
	content := []byte("Call +79509191919")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for empty rules, got %d", len(result.Entries))
	}
}

func TestRegexMask_InvalidRegex(t *testing.T) {
	cfg := MaskingConfig{
		Mode: MaskModeLLMOnly,
		Rules: []MaskingRule{
			{Name: "bad", Pattern: `[invalid`, Builtin: false},
			PresetPhoneRule,
		},
	}
	cfg.Compile()
	content := []byte("Call +79509191919")
	result := ApplyMasking(content, &cfg)
	// Invalid regex skipped, phone rule still works
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (skip invalid regex), got %d", len(result.Entries))
	}
}

func TestUnmask_RestoresOriginals(t *testing.T) {
	cfg := MaskingConfig{
		Mode:  MaskModeLLMOnly,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	cfg.Compile()
	original := "Call +79509191919 please"
	result := ApplyMasking([]byte(original), &cfg)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	unmasked := UnmaskContent(result.Content, result.Entries)
	if string(unmasked) != original {
		t.Errorf("unmask round-trip failed: got %q, want %q", string(unmasked), original)
	}
}

func TestRegexMask_NoMatch(t *testing.T) {
	cfg := MaskingConfig{
		Mode:  MaskModeLLMOnly,
		Rules: []MaskingRule{PresetPhoneRule},
	}
	cfg.Compile()
	content := []byte("hello world 12345")
	result := ApplyMasking(content, &cfg)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result.Entries))
	}
	if string(result.Content) != "hello world 12345" {
		t.Errorf("content should be unchanged, got %s", string(result.Content))
	}
}
