package masking

import (
	"strings"
	"testing"
)

func TestPhoneMask_PlainDigits(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("79383293838")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Original != "79383293838" {
		t.Errorf("expected original 79383293838, got %s", entries[0].Original)
	}
	if string(result) == "79383293838" {
		t.Error("content should be masked but was unchanged")
	}
	// First digit preserved as '7', rest masked
	if entries[0].Masked != "71111111111" {
		t.Errorf("expected masked 71111111111, got %s", entries[0].Masked)
	}
}

func TestPhoneMask_WithPlus(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("+79383293838")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Original != "+79383293838" {
		t.Errorf("expected original +79383293838, got %s", entries[0].Original)
	}
	// Plus preserved, digits masked
	expected := "+71111111111"
	if entries[0].Masked != expected {
		t.Errorf("expected masked %s, got %s", expected, entries[0].Masked)
	}
	if string(result) != expected {
		t.Errorf("expected result %s, got %s", expected, string(result))
	}
}

func TestPhoneMask_WithSpaces(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("+7 938 329 38 38")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	expected := "+7 111 111 11 11"
	if entries[0].Masked != expected {
		t.Errorf("expected masked %q, got %q", expected, entries[0].Masked)
	}
	if string(result) != expected {
		t.Errorf("expected result %q, got %q", expected, string(result))
	}
}

func TestPhoneMask_WithDashes(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("7-839-238-337-72")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	expected := "7-111-111-111-11"
	if entries[0].Masked != expected {
		t.Errorf("expected masked %q, got %q", expected, entries[0].Masked)
	}
	if string(result) != expected {
		t.Errorf("expected result %q, got %q", expected, string(result))
	}
}

func TestPhoneMask_MixedFormatting(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("(938) 329-3838")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// First digit '9' replaced with '7', parens preserved
	expected := "(711) 111-1111"
	if entries[0].Masked != expected {
		t.Errorf("expected masked %q, got %q", expected, entries[0].Masked)
	}
	if string(result) != expected {
		t.Errorf("expected result %q, got %q", expected, string(result))
	}
}

func TestPhoneMask_MultipleNumbers(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("Call 79383293838 or 79991234567 please")
	_, entries := m.Mask(content, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Original != "79383293838" {
		t.Errorf("first original: expected 79383293838, got %s", entries[0].Original)
	}
	if entries[1].Original != "79991234567" {
		t.Errorf("second original: expected 79991234567, got %s", entries[1].Original)
	}
}

func TestPhoneMask_EmbeddedInProse(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("Call me at 79383293838 or email me")
	result, entries := m.Mask(content, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	resultStr := string(result)
	if !strings.HasPrefix(resultStr, "Call me at ") {
		t.Errorf("prose prefix not preserved: %s", resultStr)
	}
	if !strings.HasSuffix(resultStr, " or email me") {
		t.Errorf("prose suffix not preserved: %s", resultStr)
	}
	if strings.Contains(resultStr, "79383293838") {
		t.Error("original phone number still present in result")
	}
}

func TestPhoneMask_NoMatch(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("hello world 12345")
	result, entries := m.Mask(content, 0)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if string(result) != "hello world 12345" {
		t.Errorf("content should be unchanged, got %s", string(result))
	}
}

func TestPhoneMask_SequentialCounters(t *testing.T) {
	m := &PhoneMasker{}
	content := []byte("A: 79383293838, B: 79991234567, C: 78001112233")
	_, entries := m.Mask(content, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Counter 0 -> suffix "1", counter 1 -> suffix "2", counter 2 -> suffix "3"
	if !strings.HasSuffix(entries[0].Masked, "1") {
		t.Errorf("first entry should end with 1, got %s", entries[0].Masked)
	}
	if !strings.HasSuffix(entries[1].Masked, "2") {
		t.Errorf("second entry should end with 2, got %s", entries[1].Masked)
	}
	if !strings.HasSuffix(entries[2].Masked, "3") {
		t.Errorf("third entry should end with 3, got %s", entries[2].Masked)
	}
}

func TestPhoneMask_LengthPreservation(t *testing.T) {
	cases := []string{
		"79383293838",
		"+79383293838",
		"+7 938 329 38 38",
		"7-839-238-337-72",
		"(938) 329-3838",
	}
	m := &PhoneMasker{}
	for _, tc := range cases {
		_, entries := m.Mask([]byte(tc), 0)
		if len(entries) != 1 {
			t.Errorf("case %q: expected 1 entry, got %d", tc, len(entries))
			continue
		}
		if len(entries[0].Masked) != len(tc) {
			t.Errorf("case %q: length mismatch: original=%d, masked=%d (%s)",
				tc, len(tc), len(entries[0].Masked), entries[0].Masked)
		}
	}
}

func TestUnmask_RestoresOriginals(t *testing.T) {
	m := &PhoneMasker{}
	original := "Call 79383293838 please"
	result, entries := m.Mask([]byte(original), 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Simulate LLM response containing masked value
	response := result
	unmasked := UnmaskContent(response, entries)
	if string(unmasked) != original {
		t.Errorf("unmask round-trip failed: got %q, want %q", string(unmasked), original)
	}
}

func TestUnmask_MultipleValues(t *testing.T) {
	m := &PhoneMasker{}
	original := "Call 79383293838, or 79991234567, or 78001112233"
	_, entries := m.Mask([]byte(original), 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Build a response that contains all three masked values
	response := []byte("Numbers: " + entries[0].Masked + " and " + entries[1].Masked + " and " + entries[2].Masked)
	unmasked := UnmaskContent(response, entries)
	unmaskedStr := string(unmasked)
	if !strings.Contains(unmaskedStr, "79383293838") {
		t.Error("first original not restored")
	}
	if !strings.Contains(unmaskedStr, "79991234567") {
		t.Error("second original not restored")
	}
	if !strings.Contains(unmaskedStr, "78001112233") {
		t.Error("third original not restored")
	}
}
