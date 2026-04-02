package handler

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// extractInputText converts a raw LLM request body into human-readable "role: content" lines.
// For OpenAI-compatible format: iterates the "messages" array.
// For Anthropic format: prepends the top-level "system" field if present, then iterates "messages".
// Content can be a string or an array of content blocks (type+text). Text blocks are concatenated.
// On any parse error, returns the raw body string as-is (no data loss).
func extractInputText(inputBody []byte, providerType string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(inputBody, &raw); err != nil {
		return "[unparseable request body]"
	}

	var sb strings.Builder

	// Anthropic top-level system field
	if providerType == "anthropic" {
		if systemRaw, ok := raw["system"]; ok {
			var system string
			if err := json.Unmarshal(systemRaw, &system); err == nil && system != "" {
				sb.WriteString("system: ")
				sb.WriteString(system)
				sb.WriteByte('\n')
			} else {
				// Anthropic supports system as array of content blocks
				systemText := extractContentField(systemRaw)
				if systemText != "" {
					sb.WriteString("system: ")
					sb.WriteString(systemText)
					sb.WriteByte('\n')
				}
			}
		}
	}

	// Messages array
	messagesRaw, ok := raw["messages"]
	if !ok {
		return sb.String()
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return "[unparseable request body]"
	}

	for _, msgRaw := range messages {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &msg); err != nil {
			continue
		}

		var role string
		if roleRaw, ok := msg["role"]; ok {
			if err := json.Unmarshal(roleRaw, &role); err != nil {
				slog.Warn("extract: failed to parse message role", "error", err)
			}
		}

		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}

		contentText := extractContentField(contentRaw)
		if contentText == "" {
			continue
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(contentText)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// extractContentField handles the "content" field of a message, which may be:
//   - a JSON string: returned as-is
//   - an array of content blocks: text blocks are concatenated
func extractContentField(contentRaw json.RawMessage) string {
	// Try string first
	var contentStr string
	if err := json.Unmarshal(contentRaw, &contentStr); err == nil {
		return contentStr
	}

	// Try content block array
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, block := range blocks {
		typeRaw, hasType := block["type"]
		if !hasType {
			continue
		}
		var blockType string
		if err := json.Unmarshal(typeRaw, &blockType); err != nil {
			slog.Warn("extract: failed to parse content block type", "error", err)
			continue
		}
		if blockType != "text" {
			continue
		}
		textRaw, ok := block["text"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(textRaw, &text); err != nil {
			slog.Warn("extract: failed to parse content block text", "error", err)
		} else {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(text)
		}
	}
	return sb.String()
}

// extractOutputText converts a raw LLM response into clean assistant text.
//
// For non-SSE responses:
//   - OpenAI-compatible: extracts choices[0].message.content
//   - Anthropic: concatenates text blocks from the "content" array
//
// For SSE responses:
//   - Splits on newlines, finds "data: {" lines
//   - OpenAI SSE: extracts choices[0].delta.content from each chunk
//   - Anthropic SSE: extracts delta.text from content_block_delta events
//
// On any parse error, returns outputStr as-is (no data loss).
func extractOutputText(outputStr string, providerType string, isSSE bool) string {
	if isSSE {
		return extractSSEOutputText(outputStr, providerType)
	}
	return extractNonSSEOutputText(outputStr, providerType)
}

func extractNonSSEOutputText(outputStr string, providerType string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(outputStr), &raw); err != nil {
		return "[unparseable response body]"
	}

	if providerType == "anthropic" {
		return extractAnthropicOutputText(raw, outputStr)
	}
	return extractOpenAIOutputText(raw, outputStr)
}

func extractOpenAIOutputText(raw map[string]json.RawMessage, fallback string) string {
	choicesRaw, ok := raw["choices"]
	if !ok {
		return ""
	}
	var choices []map[string]json.RawMessage
	if err := json.Unmarshal(choicesRaw, &choices); err != nil || len(choices) == 0 {
		return ""
	}

	msgRaw, ok := choices[0]["message"]
	if !ok {
		return ""
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return ""
	}

	contentRaw, ok := msg["content"]
	if !ok {
		return ""
	}

	var content string
	if err := json.Unmarshal(contentRaw, &content); err != nil {
		// content may be null — return empty
		return ""
	}
	return content
}

func extractAnthropicOutputText(raw map[string]json.RawMessage, fallback string) string {
	contentRaw, ok := raw["content"]
	if !ok {
		return ""
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, block := range blocks {
		typeRaw, ok := block["type"]
		if !ok {
			continue
		}
		var blockType string
		if err := json.Unmarshal(typeRaw, &blockType); err != nil {
			slog.Warn("extract: failed to parse anthropic output block type", "error", err)
			continue
		}
		if blockType != "text" {
			continue
		}
		textRaw, ok := block["text"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(textRaw, &text); err != nil {
			slog.Warn("extract: failed to parse anthropic output block text", "error", err)
		} else {
			sb.WriteString(text)
		}
	}
	return sb.String()
}

func extractSSEOutputText(outputStr string, providerType string) string {
	var sb strings.Builder

	for _, line := range strings.Split(outputStr, "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			continue
		}

		if providerType == "anthropic" {
			// Only process content_block_delta events
			typeRaw, ok := raw["type"]
			if !ok {
				continue
			}
			var eventType string
			if err := json.Unmarshal(typeRaw, &eventType); err != nil {
				slog.Warn("extract: failed to parse SSE event type", "error", err)
				continue
			}
			if eventType != "content_block_delta" {
				continue
			}

			deltaRaw, ok := raw["delta"]
			if !ok {
				continue
			}
			var delta map[string]json.RawMessage
			if err := json.Unmarshal(deltaRaw, &delta); err != nil {
				continue
			}
			textRaw, ok := delta["text"]
			if !ok {
				continue
			}
			var text string
			if err := json.Unmarshal(textRaw, &text); err == nil {
				sb.WriteString(text)
			}
		} else {
			// OpenAI-compatible SSE: choices[0].delta.content
			choicesRaw, ok := raw["choices"]
			if !ok {
				continue
			}
			var choices []map[string]json.RawMessage
			if err := json.Unmarshal(choicesRaw, &choices); err != nil || len(choices) == 0 {
				continue
			}
			deltaRaw, ok := choices[0]["delta"]
			if !ok {
				continue
			}
			var delta map[string]json.RawMessage
			if err := json.Unmarshal(deltaRaw, &delta); err != nil {
				continue
			}
			contentRaw, ok := delta["content"]
			if !ok {
				continue
			}
			var content string
			if err := json.Unmarshal(contentRaw, &content); err == nil {
				sb.WriteString(content)
			}
		}
	}

	return sb.String()
}
