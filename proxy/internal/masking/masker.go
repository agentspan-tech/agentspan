package masking

import "bytes"

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

// MaskingConfig holds per-type masking modes. Extensible for future types.
type MaskingConfig struct {
	Phone MaskMode `json:"phone"`
}

// Masker is the interface for a PII masker implementation.
type Masker interface {
	Type() MaskType
	Mask(content []byte, startIndex int) ([]byte, []MaskEntry)
}

// getModeForType returns the MaskMode for a given type from config, defaulting to "off".
func getModeForType(config MaskingConfig, t MaskType) MaskMode {
	switch t {
	case MaskTypePhone:
		if config.Phone == "" {
			return MaskModeOff
		}
		return config.Phone
	default:
		return MaskModeOff
	}
}

// ApplyMasking iterates maskers, skips those whose mode is "off",
// and aggregates entries with globally unique startIndex counters.
func ApplyMasking(content []byte, config MaskingConfig, maskers []Masker) *MaskResult {
	if len(content) == 0 {
		return &MaskResult{Content: content, Entries: nil}
	}

	var allEntries []MaskEntry
	current := content

	for _, m := range maskers {
		mode := getModeForType(config, m.Type())
		if mode == MaskModeOff {
			continue
		}
		masked, entries := m.Mask(current, len(allEntries))
		current = masked
		allEntries = append(allEntries, entries...)
	}

	return &MaskResult{Content: current, Entries: allEntries}
}

// UnmaskContent reverses masking by replacing each masked value with its original.
func UnmaskContent(content []byte, entries []MaskEntry) []byte {
	result := content
	for _, e := range entries {
		result = bytes.ReplaceAll(result, []byte(e.Masked), []byte(e.Original))
	}
	return result
}
