package service

import (
	"strings"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
)

// TestExtractSystemBlock tests the unexported extractSystemBlock helper.
func TestExtractSystemBlock(t *testing.T) {
	t.Run("single system line", func(t *testing.T) {
		input := "system: You are a helpful assistant.\nuser: Hello\n"
		got := extractSystemBlock(input)
		if got != "system: You are a helpful assistant.\n" {
			t.Errorf("expected %q, got %q", "system: You are a helpful assistant.\n", got)
		}
	})

	t.Run("multiple system lines", func(t *testing.T) {
		input := "system: You are helpful.\nsystem: Be concise.\nuser: Hi\n"
		got := extractSystemBlock(input)
		want := "system: You are helpful.\nsystem: Be concise.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("no system lines returns empty", func(t *testing.T) {
		input := "user: Hello\nassistant: Hi\n"
		got := extractSystemBlock(input)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		got := extractSystemBlock("")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("system only without trailing newline", func(t *testing.T) {
		input := "system: You are helpful."
		got := extractSystemBlock(input)
		if got != "system: You are helpful." {
			t.Errorf("expected %q, got %q", "system: You are helpful.", got)
		}
	})

	t.Run("system line followed by user line stops at user", func(t *testing.T) {
		input := "system: Be helpful and precise in your responses always.\nuser: What is Go?\nassistant: Go is a language.\n"
		got := extractSystemBlock(input)
		want := "system: Be helpful and precise in your responses always.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("user message that happens to start with system word is not extracted", func(t *testing.T) {
		input := "user: system: is broken\n"
		got := extractSystemBlock(input)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// TestExtractSystemBlockCandidate tests the candidate extraction logic.
func TestExtractSystemBlockCandidate(t *testing.T) {
	mkSpan := func(input string) db.GetSpansBySessionIDRow {
		return db.GetSpansBySessionIDRow{Input: &input}
	}

	t.Run("all spans share same system block", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{
			mkSpan("system: You are helpful.\nuser: Hello\n"),
			mkSpan("system: You are helpful.\nuser: World\n"),
		}
		got := extractSystemBlockCandidate(spans)
		if got != "system: You are helpful.\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("spans with broken input are skipped", func(t *testing.T) {
		broken := "[unparseable request body]"
		spans := []db.GetSpansBySessionIDRow{
			mkSpan("system: You are helpful.\nuser: Hello\n"),
			{Input: &broken},
			mkSpan("system: You are helpful.\nuser: World\n"),
		}
		got := extractSystemBlockCandidate(spans)
		if got != "system: You are helpful.\n" {
			t.Errorf("expected system block, got %q", got)
		}
	})

	t.Run("single parseable span returns system block", func(t *testing.T) {
		broken := "[unparseable request body]"
		spans := []db.GetSpansBySessionIDRow{
			mkSpan("system: You are helpful.\nuser: Hello\n"),
			{Input: &broken},
		}
		got := extractSystemBlockCandidate(spans)
		if got != "system: You are helpful.\n" {
			t.Errorf("expected system block, got %q", got)
		}
	})

	t.Run("differing system blocks returns empty", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{
			mkSpan("system: You are helpful.\nuser: Hello\n"),
			mkSpan("system: Be concise.\nuser: World\n"),
		}
		got := extractSystemBlockCandidate(spans)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

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

	t.Run("identical strings returns full string", func(t *testing.T) {
		s := "identical string content here"
		got := longestCommonPrefix(s, s)
		if got != s {
			t.Errorf("expected full string %q, got %q", s, got)
		}
	})

	t.Run("one empty string returns empty string", func(t *testing.T) {
		got := longestCommonPrefix("", "abc")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("multi-byte UTF-8 splits on rune boundary", func(t *testing.T) {
		a := "日本語テスト"
		b := "日本人テスト"
		got := longestCommonPrefix(a, b)
		if got != "日本" {
			t.Errorf("expected %q, got %q", "日本", got)
		}
	})
}

// TestBuildMetadataSummary tests the unexported buildMetadataSummary helper.
func TestBuildMetadataSummary(t *testing.T) {
	makeSpan := func(model string, inputTokens, outputTokens int32) db.GetSpansBySessionIDRow {
		return db.GetSpansBySessionIDRow{
			Model:        model,
			InputTokens:  &inputTokens,
			OutputTokens: &outputTokens,
		}
	}

	t.Run("single span with status", func(t *testing.T) {
		inputTokens := int32(300)
		outputTokens := int32(200)
		spans := []db.GetSpansBySessionIDRow{
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
		spans := []db.GetSpansBySessionIDRow{
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
		spans := []db.GetSpansBySessionIDRow{makeSpan("gpt-4o", 7000, 5400)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "12.4k tokens") {
			t.Errorf("expected '12.4k tokens' in %q", got)
		}
	})

	t.Run("empty spans handles gracefully", func(t *testing.T) {
		got := buildMetadataSummary([]db.GetSpansBySessionIDRow{}, "")
		if got == "" {
			t.Error("expected non-empty result for zero spans")
		}
		if !strings.Contains(got, "0 spans") {
			t.Errorf("expected '0 spans' in %q", got)
		}
	})

	t.Run("nil token fields treated as zero", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{
			{Model: "gpt-4", InputTokens: nil, OutputTokens: nil},
		}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "0 tokens") {
			t.Errorf("expected '0 tokens' in %q", got)
		}
	})

	t.Run("more than 3 models truncated to 3 in display", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{
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
		spans := []db.GetSpansBySessionIDRow{makeSpan("gpt-4", 500, 500)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "1.0k tokens") {
			t.Errorf("expected '1.0k tokens' in %q", got)
		}
	})

	t.Run("999 tokens formatted as integer", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{makeSpan("gpt-4", 500, 499)}
		got := buildMetadataSummary(spans, "")
		if !strings.Contains(got, "999 tokens") {
			t.Errorf("expected '999 tokens' in %q", got)
		}
	})

	t.Run("no status omits trailing comma", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{makeSpan("gpt-4", 10, 5)}
		got := buildMetadataSummary(spans, "")
		if strings.HasSuffix(got, ", ") || strings.HasSuffix(got, ",") {
			t.Errorf("trailing comma in %q", got)
		}
	})

	t.Run("duplicate models counted once", func(t *testing.T) {
		spans := []db.GetSpansBySessionIDRow{
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

// TestParseSpanVerdicts tests the LLM response parser.
func TestParseSpanVerdicts(t *testing.T) {
	t.Run("valid JSON array", func(t *testing.T) {
		resp := `[{"span_index": 0, "ok": true}, {"span_index": 1, "ok": false, "reason": "Output echoes input"}]`
		verdicts, err := parseSpanVerdicts(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(verdicts) != 2 {
			t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
		}
		if !verdicts[0].OK {
			t.Error("expected span 0 to be ok")
		}
		if verdicts[1].OK {
			t.Error("expected span 1 to not be ok")
		}
		if verdicts[1].Reason != "Output echoes input" {
			t.Errorf("expected reason %q, got %q", "Output echoes input", verdicts[1].Reason)
		}
	})

	t.Run("JSON wrapped in markdown code fences", func(t *testing.T) {
		resp := "```json\n[{\"span_index\": 0, \"ok\": true}]\n```"
		verdicts, err := parseSpanVerdicts(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(verdicts) != 1 {
			t.Fatalf("expected 1 verdict, got %d", len(verdicts))
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := parseSpanVerdicts("not json at all")
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("all ok verdicts", func(t *testing.T) {
		resp := `[{"span_index": 0, "ok": true}, {"span_index": 1, "ok": true}]`
		verdicts, err := parseSpanVerdicts(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, v := range verdicts {
			if !v.OK {
				t.Errorf("expected span %d to be ok", v.SpanIndex)
			}
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
