package provider

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// StreamDelta represents content and thinking deltas from a stream chunk
type StreamDelta struct {
	Content  string
	Thinking string
}

// StreamResult holds the accumulated result from parsing a stream
type StreamResult struct {
	Content   string
	Thinking  string
	ToolCalls []ToolCall
	Usage     Usage
}

// streamToolCall is used to accumulate partial tool calls from stream
type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ParseSSEStream reads an OpenAI-compatible SSE stream and calls onChunk for each content/thinking delta.
// Stops on [DONE] or stream end. Returns accumulated content, thinking, and tool_calls.
func ParseSSEStream(stream io.ReadCloser, onChunk func(StreamDelta)) (*StreamResult, error) {
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(nil, 512*1024) // 512KB for long lines

	var content, thinking string
	toolCallsByIndex := make(map[int]*ToolCall)
	var usage Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string           `json:"content"`
					Thinking  string           `json:"thinking"`
					ToolCalls []streamToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil && (chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
			usage = *chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta

		if d.Content != "" || d.Thinking != "" {
			content += d.Content
			thinking += d.Thinking
			if onChunk != nil {
				onChunk(StreamDelta{Content: d.Content, Thinking: d.Thinking})
			}
		}

		for _, tc := range d.ToolCalls {
			if tc.Index >= 0 {
				if _, ok := toolCallsByIndex[tc.Index]; !ok {
					toolCallsByIndex[tc.Index] = &ToolCall{Type: "function"}
				}
				cur := toolCallsByIndex[tc.Index]
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Function.Name != "" {
					cur.Function.Name = tc.Function.Name
				}
				cur.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	var toolCalls []ToolCall
	for i := 0; ; i++ {
		if tc, ok := toolCallsByIndex[i]; ok {
			toolCalls = append(toolCalls, *tc)
		} else {
			break
		}
	}

	return &StreamResult{
		Content:   content,
		Thinking:  thinking,
		ToolCalls: toolCalls,
		Usage:     usage,
	}, scanner.Err()
}
