package masking

import (
	"testing"
)

func TestApplyMasking_OffMode(t *testing.T) {
	content := []byte("Call +79383293838")
	config := MaskingConfig{Mode: MaskModeOff, Rules: []MaskingRule{PresetPhoneRule}}

	result := ApplyMasking(content, &config)
	if string(result.Content) != string(content) {
		t.Errorf("off mode should return unchanged content")
	}
	if len(result.Entries) != 0 {
		t.Errorf("off mode should produce no entries")
	}
}

func TestApplyMasking_EmptyContent(t *testing.T) {
	config := MaskingConfig{Mode: MaskModeLLMOnly, Rules: []MaskingRule{PresetPhoneRule}}

	result := ApplyMasking(nil, &config)
	if result.Entries != nil {
		t.Errorf("expected nil entries for nil content, got %d", len(result.Entries))
	}

	result = ApplyMasking([]byte{}, &config)
	if result.Entries != nil {
		t.Errorf("expected nil entries for empty content, got %d", len(result.Entries))
	}
}

func TestApplyMasking_EmptyMode(t *testing.T) {
	config := MaskingConfig{Mode: "", Rules: []MaskingRule{PresetPhoneRule}}
	content := []byte("Call +79383293838")

	result := ApplyMasking(content, &config)
	if len(result.Entries) != 0 {
		t.Errorf("empty mode should default to off, got %d entries", len(result.Entries))
	}
}

func TestApplyMasking_NoRules(t *testing.T) {
	config := MaskingConfig{Mode: MaskModeLLMOnly, Rules: nil}
	content := []byte("Call +79383293838")

	result := ApplyMasking(content, &config)
	if len(result.Entries) != 0 {
		t.Errorf("no rules should produce no entries, got %d", len(result.Entries))
	}
}

func TestUnmaskContent_NoEntries(t *testing.T) {
	content := []byte("Hello world")
	result := UnmaskContent(content, nil)
	if string(result) != "Hello world" {
		t.Errorf("no entries should return unchanged content")
	}
}

func TestRoundTrip_MaskThenUnmask(t *testing.T) {
	original := []byte("Call me at +79383293838 or +79991234567")
	config := MaskingConfig{Mode: MaskModeLLMOnly, Rules: []MaskingRule{PresetPhoneRule}}

	config.Compile()
	masked := ApplyMasking(original, &config)
	if len(masked.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(masked.Entries))
	}

	restored := UnmaskContent(masked.Content, masked.Entries)
	if string(restored) != string(original) {
		t.Errorf("round-trip failed:\n  original: %q\n  restored: %q", string(original), string(restored))
	}
}
