package service

import (
	"strings"
	"testing"

	"github.com/agentspan/processing/internal/db"
)

// TestLongestCommonPrefix tests the unexported longestCommonPrefix helper.
func TestLongestCommonPrefix(t *testing.T) {
	t.Run("shared prefix of 150 chars returns 150-char prefix", func(t *testing.T) {
		prefix := strings.Repeat("a", 150)
		a := prefix + "xxxxxxxxxxx"
		b := prefix + "yyyyyyyyyyy"
		got := longestCommonPrefix(a, b)
		if got != prefix {
			t.Errorf("expected prefix of length 150, got length %d", len(got))
		}
	})

	t.Run("no common prefix returns empty string", func(t *testing.T) {
		got := longestCommonPrefix("abc", "xyz")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("common prefix shorter than 100 chars returns short prefix", func(t *testing.T) {
		a := "hello world extra"
		b := "hello world other"
		got := longestCommonPrefix(a, b)
		if got != "hello world " {
			t.Errorf("expected %q, got %q", "hello world ", got)
		}
	})

	t.Run("one empty string returns empty string", func(t *testing.T) {
		got := longestCommonPrefix("", "abc")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
		got = longestCommonPrefix("abc", "")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("identical strings returns full string", func(t *testing.T) {
		s := "identical string content here"
		got := longestCommonPrefix(s, s)
		if got != s {
			t.Errorf("expected full string %q, got %q", s, got)
		}
	})

	t.Run("multi-byte UTF-8 splits on rune boundary", func(t *testing.T) {
		// "日本語" is 3 runes, 9 bytes. Both strings share the first 2 runes.
		a := "日本語テスト"
		b := "日本人テスト"
		got := longestCommonPrefix(a, b)
		if got != "日本" {
			t.Errorf("expected %q, got %q", "日本", got)
		}
	})

	t.Run("emoji rune boundary", func(t *testing.T) {
		a := "Hello 🌍 world"
		b := "Hello 🌎 world"
		got := longestCommonPrefix(a, b)
		if got != "Hello " {
			t.Errorf("expected %q, got %q", "Hello ", got)
		}
	})

	t.Run("both empty strings returns empty", func(t *testing.T) {
		got := longestCommonPrefix("", "")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// TestBuildMetadataSummary tests the unexported buildMetadataSummary helper.
func TestBuildMetadataSummary(t *testing.T) {
	makeSpan := func(model string, inputTokens, outputTokens int32) db.Span {
		return db.Span{
			Model:        model,
			InputTokens:  &inputTokens,
			OutputTokens: &outputTokens,
		}
	}

	t.Run("single span with status", func(t *testing.T) {
		inputTokens := int32(300)
		outputTokens := int32(200)
		spans := []db.Span{
			{
				Model:        "gpt-4o",
				InputTokens:  &inputTokens,
				OutputTokens: &outputTokens,
			},
		}
		got := buildMetadataSummary(spans, "completed")
		expected := "1 span, 1 model (gpt-4o), 500 tokens, completed"
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("multiple spans with different models counts and lists correctly", func(t *testing.T) {
		spans := []db.Span{
			makeSpan("gpt-4o", 100, 50),
			makeSpan("claude-3-haiku", 200, 100),
			makeSpan("gpt-4o", 50, 25),
		}
		got := buildMetadataSummary(spans, "completed")
		if !strings.Contains(got, "3 spans") {
			t.Errorf("expected '3 spans' in %q", got)
		}
		if !strings.Contains(got, "2 models") {
			t.Errorf("expected '2 models' in %q", got)
		}
		if !strings.Contains(got, "gpt-4o") {
			t.Errorf("expected 'gpt-4o' in %q", got)
		}
		if !strings.Contains(got, "claude-3-haiku") {
			t.Errorf("expected 'claude-3-haiku' in %q", got)
		}
	})

	t.Run("token count >= 1000 formatted as X.Xk", func(t *testing.T) {
		spans := []db.Span{makeSpan("gpt-4o", 7000, 5400)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "12.4k tokens") {
			t.Errorf("expected '12.4k tokens' in %q", got)
		}
	})

	t.Run("empty spans handles gracefully", func(t *testing.T) {
		got := buildMetadataSummary([]db.Span{}, "")
		if got == "" {
			t.Error("expected non-empty result for zero spans")
		}
		if !strings.Contains(got, "0 spans") {
			t.Errorf("expected '0 spans' in %q", got)
		}
	})

	t.Run("nil token fields treated as zero", func(t *testing.T) {
		spans := []db.Span{
			{Model: "gpt-4", InputTokens: nil, OutputTokens: nil},
		}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "0 tokens") {
			t.Errorf("expected '0 tokens' in %q", got)
		}
	})

	t.Run("more than 3 models truncated to 3 in display", func(t *testing.T) {
		spans := []db.Span{
			makeSpan("model-a", 10, 5),
			makeSpan("model-b", 10, 5),
			makeSpan("model-c", 10, 5),
			makeSpan("model-d", 10, 5),
		}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "4 models") {
			t.Errorf("expected '4 models' in %q", got)
		}
		// Should show first 3 models but not the 4th.
		if !strings.Contains(got, "model-a") || !strings.Contains(got, "model-b") || !strings.Contains(got, "model-c") {
			t.Errorf("expected first 3 models in %q", got)
		}
		if strings.Contains(got, "model-d") {
			t.Errorf("did not expect model-d in display, got %q", got)
		}
	})

	t.Run("exactly 1000 tokens formatted as 1.0k", func(t *testing.T) {
		spans := []db.Span{makeSpan("gpt-4", 500, 500)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "1.0k tokens") {
			t.Errorf("expected '1.0k tokens' in %q", got)
		}
	})

	t.Run("999 tokens formatted as integer", func(t *testing.T) {
		spans := []db.Span{makeSpan("gpt-4", 500, 499)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "999 tokens") {
			t.Errorf("expected '999 tokens' in %q", got)
		}
	})

	t.Run("no status omits trailing comma", func(t *testing.T) {
		spans := []db.Span{makeSpan("gpt-4", 10, 5)}
		got := buildMetadataSummary(spans, "")
		if strings.HasSuffix(got, ", ") || strings.HasSuffix(got, ",") {
			t.Errorf("trailing comma in %q", got)
		}
	})

	t.Run("duplicate models counted once", func(t *testing.T) {
		spans := []db.Span{
			makeSpan("gpt-4", 10, 5),
			makeSpan("gpt-4", 20, 10),
			makeSpan("gpt-4", 30, 15),
		}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "1 model") {
			t.Errorf("expected '1 model' in %q", got)
		}
	})
}

// TestTruncate tests the unexported truncate helper.
func TestTruncate(t *testing.T) {
	t.Run("string shorter than maxLen returns unchanged", func(t *testing.T) {
		s := "hello"
		got := truncate(s, 10)
		if got != s {
			t.Errorf("expected %q, got %q", s, got)
		}
	})

	t.Run("string longer than maxLen is truncated to maxLen", func(t *testing.T) {
		s := "hello world"
		got := truncate(s, 5)
		if got != "hello" {
			t.Errorf("expected %q, got %q", "hello", got)
		}
		if len(got) != 5 {
			t.Errorf("expected length 5, got %d", len(got))
		}
	})

	t.Run("empty string returns empty string", func(t *testing.T) {
		got := truncate("", 10)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("exactly maxLen returns unchanged", func(t *testing.T) {
		s := "hello"
		got := truncate(s, 5)
		if got != s {
			t.Errorf("expected %q, got %q", s, got)
		}
	})

	t.Run("multi-byte UTF-8 truncates on rune boundary", func(t *testing.T) {
		// "日本語" = 9 bytes (3 bytes per rune). Truncating at 7 should back up to 6 (2 full runes).
		s := "日本語"
		got := truncate(s, 7)
		if got != "日本" {
			t.Errorf("expected %q, got %q", "日本", got)
		}
	})

	t.Run("truncate mid-emoji backs up to rune boundary", func(t *testing.T) {
		// "A🌍B" = 1 + 4 + 1 = 6 bytes. Truncating at 3 lands mid-emoji, should back up to 1.
		s := "A🌍B"
		got := truncate(s, 3)
		if got != "A" {
			t.Errorf("expected %q, got %q", "A", got)
		}
	})

	t.Run("maxLen zero returns empty", func(t *testing.T) {
		got := truncate("hello", 0)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// TestDeref tests the unexported deref helper.
func TestDeref(t *testing.T) {
	t.Run("nil pointer returns empty string", func(t *testing.T) {
		got := deref(nil)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("non-nil pointer returns value", func(t *testing.T) {
		s := "hello"
		got := deref(&s)
		if got != s {
			t.Errorf("expected %q, got %q", s, got)
		}
	})
}
