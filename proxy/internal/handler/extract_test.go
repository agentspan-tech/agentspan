package handler

import (
	"testing"
)

func TestExtractInputText(t *testing.T) {
	tests := []struct {
		name         string
		inputBody    string
		providerType string
		want         string
	}{
		{
			name:         "OpenAI messages array",
			inputBody:    `{"messages":[{"role":"system","content":"You are helpful"},{"role":"user","content":"Hello"}]}`,
			providerType: "openai",
			want:         "system: You are helpful\nuser: Hello\n",
		},
		{
			name:         "Anthropic system field plus messages",
			inputBody:    `{"system":"You are helpful","messages":[{"role":"user","content":"Hello"}]}`,
			providerType: "anthropic",
			want:         "system: You are helpful\nuser: Hello\n",
		},
		{
			name:         "Anthropic content array blocks",
			inputBody:    `{"messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`,
			providerType: "anthropic",
			want:         "user: Hello\n",
		},
		{
			name:         "Content array with multiple text blocks",
			inputBody:    `{"messages":[{"role":"user","content":[{"type":"text","text":"Part 1"},{"type":"image","text":"ignored"},{"type":"text","text":"Part 2"}]}]}`,
			providerType: "openai",
			want:         "user: Part 1 Part 2\n",
		},
		{
			name:         "Invalid JSON returns placeholder",
			inputBody:    `not valid json`,
			providerType: "openai",
			want:         "[unparseable request body]",
		},
		{
			name:         "Empty messages array",
			inputBody:    `{"messages":[]}`,
			providerType: "openai",
			want:         "",
		},
		{
			name:         "Missing messages key",
			inputBody:    `{"model":"gpt-4"}`,
			providerType: "openai",
			want:         "",
		},
		{
			name:         "Anthropic system only no messages",
			inputBody:    `{"system":"Be helpful","messages":[]}`,
			providerType: "anthropic",
			want:         "system: Be helpful\n",
		},
		{
			name:         "Content block with non-text types only",
			inputBody:    `{"messages":[{"role":"user","content":[{"type":"image","data":"base64"},{"type":"tool_use","id":"123"}]}]}`,
			providerType: "openai",
			want:         "",
		},
		{
			name:         "Anthropic system array content blocks",
			inputBody:    `{"system":[{"type":"text","text":"You are helpful"}],"messages":[{"role":"user","content":"Hello"}]}`,
			providerType: "anthropic",
			want:         "system: You are helpful\nuser: Hello\n",
		},
		{
			name:         "Anthropic system array multiple blocks",
			inputBody:    `{"system":[{"type":"text","text":"Be helpful."},{"type":"text","text":"Be concise."}],"messages":[{"role":"user","content":"Hi"}]}`,
			providerType: "anthropic",
			want:         "system: Be helpful. Be concise.\nuser: Hi\n",
		},
		{
			name:         "Invalid messages array type",
			inputBody:    `{"messages":"not_an_array"}`,
			providerType: "openai",
			want:         "[unparseable request body]",
		},
		{
			name:         "Message with no content field",
			inputBody:    `{"messages":[{"role":"user"}]}`,
			providerType: "openai",
			want:         "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractInputText([]byte(tc.inputBody), tc.providerType)
			if got != tc.want {
				t.Errorf("extractInputText() =\n  %q\nwant:\n  %q", got, tc.want)
			}
		})
	}
}

func TestExtractOutputText(t *testing.T) {
	tests := []struct {
		name         string
		outputStr    string
		providerType string
		isSSE        bool
		want         string
	}{
		{
			name:         "Non-SSE OpenAI response",
			outputStr:    `{"choices":[{"message":{"role":"assistant","content":"Hi there"}}]}`,
			providerType: "openai",
			isSSE:        false,
			want:         "Hi there",
		},
		{
			name:         "Non-SSE Anthropic response",
			outputStr:    `{"content":[{"type":"text","text":"Hi there"}]}`,
			providerType: "anthropic",
			isSSE:        false,
			want:         "Hi there",
		},
		{
			name:         "Non-SSE Anthropic multiple text blocks",
			outputStr:    `{"content":[{"type":"text","text":"Hello "},{"type":"tool_use","text":"ignored"},{"type":"text","text":"world"}]}`,
			providerType: "anthropic",
			isSSE:        false,
			want:         "Hello world",
		},
		{
			name:         "Non-SSE OpenAI empty choices",
			outputStr:    `{"choices":[]}`,
			providerType: "openai",
			isSSE:        false,
			want:         "",
		},
		{
			name:         "SSE OpenAI concatenated deltas",
			outputStr:    "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\ndata: [DONE]\n\n",
			providerType: "openai",
			isSSE:        true,
			want:         "Hello world",
		},
		{
			name:         "SSE Anthropic content_block_delta",
			outputStr:    "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" Claude\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
			providerType: "anthropic",
			isSSE:        true,
			want:         "Hello Claude",
		},
		{
			name:         "SSE with data: [DONE] ignored",
			outputStr:    "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n",
			providerType: "openai",
			isSSE:        true,
			want:         "Hi",
		},
		{
			name:         "Invalid JSON returns placeholder",
			outputStr:    "not valid json",
			providerType: "openai",
			isSSE:        false,
			want:         "[unparseable response body]",
		},
		{
			name:         "Non-SSE OpenAI null content",
			outputStr:    `{"choices":[{"message":{"role":"assistant","content":null}}]}`,
			providerType: "openai",
			isSSE:        false,
			want:         "",
		},
		{
			name:         "Non-SSE Anthropic empty content array",
			outputStr:    `{"content":[]}`,
			providerType: "anthropic",
			isSSE:        false,
			want:         "",
		},
		{
			name:         "Non-SSE OpenAI no choices key",
			outputStr:    `{"model":"gpt-4"}`,
			providerType: "openai",
			isSSE:        false,
			want:         "",
		},
		{
			name:         "Non-SSE Anthropic no content key",
			outputStr:    `{"model":"claude-3"}`,
			providerType: "anthropic",
			isSSE:        false,
			want:         "",
		},
		{
			name:         "SSE with no data lines",
			outputStr:    "event: ping\n\n",
			providerType: "openai",
			isSSE:        true,
			want:         "",
		},
		{
			name:         "SSE Anthropic ignores non-delta events",
			outputStr:    "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg1\"}}\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hi\"}}\ndata: {\"type\":\"message_stop\"}\n",
			providerType: "anthropic",
			isSSE:        true,
			want:         "Hi",
		},
		{
			name:         "SSE OpenAI with missing delta content",
			outputStr:    "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\ndata: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n",
			providerType: "openai",
			isSSE:        true,
			want:         "OK",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractOutputText(tc.outputStr, tc.providerType, tc.isSSE)
			if got != tc.want {
				t.Errorf("extractOutputText() =\n  %q\nwant:\n  %q", got, tc.want)
			}
		})
	}
}
