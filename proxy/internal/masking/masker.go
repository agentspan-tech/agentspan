package masking

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// MaskType identifies the category of PII being masked.
type MaskType string

const (
	MaskTypePhone MaskType = "phone"
)

// MaskMode controls how masking is applied for a given type.
type MaskMode string

const (
	MaskModeOff        MaskMode = "off"
	MaskModeLLMOnly    MaskMode = "llm_only"
	MaskModeLLMStorage MaskMode = "llm_storage"
)

// MaskEntry records one original-to-masked replacement.
type MaskEntry struct {
	MaskType MaskType `json:"mask_type"`
	Original string   `json:"original"`
	Masked   string   `json:"masked"`
}

// MaskResult holds masked content and all replacement entries.
type MaskResult struct {
	Content []byte      `json:"content"`
	Entries []MaskEntry `json:"entries"`
}

// MaskingRule defines a single masking rule with a name and regex pattern.
type MaskingRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Builtin bool   `json:"builtin"`
}

// MaskingConfig holds the masking mode and list of rules.
type MaskingConfig struct {
	// Mode is the global masking mode (off, llm_only, llm_storage).
	Mode  MaskMode      `json:"mode"`
	Rules []MaskingRule  `json:"rules"`

	// compiled holds pre-compiled regexps, populated by Compile().
	compiled []compiledRule
}

// compiledRule pairs a rule with its pre-compiled regexp.
type compiledRule struct {
	rule MaskingRule
	re   *regexp.Regexp
}

// PresetPhoneRule is the default built-in rule for Russian phone numbers (+79999999999).
var PresetPhoneRule = MaskingRule{
	Name:    "phone",
	Pattern: `\+7\d{10}`,
	Builtin: true,
}

// Compile pre-compiles all rule regexps. Call once after unmarshalling the config.
// Rules with empty or invalid patterns are silently skipped (fail-open).
func (c *MaskingConfig) Compile() {
	c.compiled = nil
	for _, rule := range c.Rules {
		if rule.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		c.compiled = append(c.compiled, compiledRule{rule: rule, re: re})
	}
}

// ApplyMasking applies all active masking rules to the content.
// Caller must call Compile() once before calling ApplyMasking; if Compile()
// was not called, the rules are silently skipped (fail-open).
func ApplyMasking(content []byte, config *MaskingConfig) *MaskResult {
	if len(content) == 0 || config.Mode == MaskModeOff || config.Mode == "" {
		return &MaskResult{Content: content, Entries: nil}
	}

	if len(config.compiled) == 0 {
		return &MaskResult{Content: content, Entries: nil}
	}

	var allEntries []MaskEntry
	current := content

	for _, m := range config.compiled {
		masked, entries := applyRegexMask(current, m)
		current = masked
		allEntries = append(allEntries, entries...)
	}

	return &MaskResult{Content: current, Entries: allEntries}
}

// maxMatchesPerRule caps the number of regex matches per rule to prevent
// degenerate patterns (e.g. `.` or `\d`) from replacing every character.
const maxMatchesPerRule = 1000

// applyRegexMask finds all matches of the regex and replaces them with masked values.
// Counter resets to 0 for each rule, so placeholders are [PHONE_1], [SSN_1], etc.
func applyRegexMask(content []byte, m compiledRule) ([]byte, []MaskEntry) {
	matches := m.re.FindAllIndex(content, maxMatchesPerRule)
	if len(matches) == 0 {
		return content, nil
	}

	// Deduplicate: same original text gets the same masked value.
	seen := make(map[string]string)
	var entries []MaskEntry
	counter := 0

	var result []byte
	prev := 0

	ruleName := m.rule.Name
	if ruleName == "" {
		ruleName = "rule"
	}
	// Sanitize rule name for placeholder: uppercase, replace spaces with underscores.
	placeholder := strings.ToUpper(strings.ReplaceAll(ruleName, " ", "_"))

	for _, loc := range matches {
		original := string(content[loc[0]:loc[1]])

		masked, exists := seen[original]
		if !exists {
			counter++
			masked = fmt.Sprintf("[%s_%d]", placeholder, counter)
			seen[original] = masked
			entries = append(entries, MaskEntry{
				MaskType: MaskType(ruleName),
				Original: original,
				Masked:   masked,
			})
		}

		result = append(result, content[prev:loc[0]]...)
		result = append(result, []byte(masked)...)
		prev = loc[1]
	}

	if len(entries) == 0 {
		return content, nil
	}

	result = append(result, content[prev:]...)
	return result, entries
}

// UnmaskContent reverses masking by replacing each masked value with its original.
func UnmaskContent(content []byte, entries []MaskEntry) []byte {
	result := content
	for _, e := range entries {
		result = bytes.ReplaceAll(result, []byte(e.Masked), []byte(e.Original))
	}
	return result
}
