package masking

import (
	"fmt"
	"regexp"
)

// phoneCandidateRe matches potential phone numbers in text.
// Optional + prefix and/or opening paren, then digits with separators, ending with digit.
var phoneCandidateRe = regexp.MustCompile(`\+?[(]?\d[\d\s\-.()\\/]{4,17}\d`)

// PhoneMasker detects and masks phone numbers in content.
type PhoneMasker struct{}

// Type returns MaskTypePhone.
func (p *PhoneMasker) Type() MaskType {
	return MaskTypePhone
}

// Mask detects phone numbers and replaces them with sequential masked values.
// startIndex is the counter offset for globally unique masked value suffixes.
func (p *PhoneMasker) Mask(content []byte, startIndex int) ([]byte, []MaskEntry) {
	matches := phoneCandidateRe.FindAllIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var entries []MaskEntry
	counter := startIndex

	// Build result by copying segments between matches.
	var result []byte
	prev := 0

	for _, loc := range matches {
		original := string(content[loc[0]:loc[1]])

		if !isValidPhoneNumber(original) {
			continue
		}

		masked := generateMaskedPhone(original, counter)
		entries = append(entries, MaskEntry{
			MaskType: MaskTypePhone,
			Original: original,
			Masked:   masked,
		})

		// Append content before this match, then the masked value.
		result = append(result, content[prev:loc[0]]...)
		result = append(result, []byte(masked)...)
		prev = loc[1]
		counter++
	}

	if len(entries) == 0 {
		return content, nil
	}

	// Append remaining content after last match.
	result = append(result, content[prev:]...)
	return result, entries
}

// isValidPhoneNumber checks if a candidate string has 7-15 digits.
func isValidPhoneNumber(s string) bool {
	c := countDigits(s)
	return c >= 7 && c <= 15
}

// countDigits counts the number of digit runes in a string.
func countDigits(s string) int {
	count := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			count++
		}
	}
	return count
}

// generateMaskedPhone creates a length-preserving masked phone number.
// Non-digit characters (spaces, dashes, parens, plus, dots) are preserved in-place.
// Digit positions: first digit -> '7', middle digits -> '1',
// last len(counterStr) digit positions -> counter digits (1-based).
func generateMaskedPhone(original string, index int) string {
	digitCount := countDigits(original)
	counterStr := fmt.Sprintf("%d", index+1) // 1-based counter

	bs := []byte(original)
	digitPos := 0
	fillEnd := digitCount - len(counterStr) // positions before counter suffix

	for i := range bs {
		if bs[i] >= '0' && bs[i] <= '9' {
			if digitPos == 0 {
				bs[i] = '7'
			} else if digitPos < fillEnd {
				bs[i] = '1'
			} else {
				// Counter suffix digit
				suffixIdx := digitPos - fillEnd
				if suffixIdx < len(counterStr) {
					bs[i] = counterStr[suffixIdx]
				} else {
					bs[i] = '1'
				}
			}
			digitPos++
		}
		// Non-digit chars are preserved as-is.
	}
	return string(bs)
}
