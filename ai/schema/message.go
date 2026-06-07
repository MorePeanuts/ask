package schema

import (
	"fmt"
	"strings"
)

type RoleType string

const (
	Assistant RoleType = "assistant"
	User      RoleType = "user"
	System    RoleType = "system"
	Tool      RoleType = "tool"
)

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ToolCall struct {
	// Index is used when there are multiple tool calls in a message.
	// In stream mode, it's used to identify the chunk of the tool call for merging.
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type TokenUsage struct {
	PromptTokens         int `json:"prompt_tokens"`
	PromptCacheHitTokens int `json:"prompt_token_hit_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	ReasoningTokens      int `json:"reasoning_tokens,omitempty"`
}

type ResponseMeta struct {
	// FinishReason is the reason why the chat response is finished.
	// It's usually "stop", "length", "tool_calls", "content_filter", "null".
	// This is defined by chat model implementation.
	FinishReason string      `json:"finish_reason,omitempty"`
	Usage        *TokenUsage `json:"usage,omitempty"`
}

type Message struct {
	Role    RoleType `json:"role"`
	Content string   `json:"content"`

	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ResponseMeta     *ResponseMeta `json:"response_meta,omitempty"`

	// only for AssistantMessage
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// only for ToolMessage
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	Extra map[string]any `json:"extra,omitempty"`
}

// String returns the string representation of the message.
// e.g.
//
//	msg := schema.UserMessage("hello world")
//	fmt.Println(msg.String()) // Output will be: `user: hello world``
//
//	msg := schema.Message{
//		Role:    schema.Tool,
//		Content: "{...}",
//		ToolCallID: "callxxxx"
//	}
//	fmt.Println(msg.String())
//	Output will be:
//		tool: {...}
//		call_id: callxxxx
func (m *Message) String() string {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "%s: %s", m.Role, m.Content)

	if len(m.ReasoningContent) > 0 {
		sb.WriteString("\nreasoning_content:\n")
		sb.WriteString(m.ReasoningContent)
	}

	if len(m.ToolCalls) > 0 {
		sb.WriteString("\ntool_calls:\n")
		for _, tc := range m.ToolCalls {
			if tc.Index != nil {
				fmt.Fprintf(sb, "index[%d]:", *tc.Index)
			}
			fmt.Fprintf(sb, "%+v\n", tc)
		}
	}

	if m.ToolCallID != "" {
		fmt.Fprintf(sb, "\ntool_call_id: %s", m.ToolCallID)
	}

	if m.ToolName != "" {
		fmt.Fprintf(sb, "\ntool_call_name: %s", m.ToolName)
	}

	if m.ResponseMeta != nil {
		fmt.Fprintf(sb, "\nfinish_reason: %s", m.ResponseMeta.FinishReason)
		if m.ResponseMeta.Usage != nil {
			fmt.Fprintf(sb, "\nusage: %v", m.ResponseMeta.Usage)
		}
	}

	return sb.String()
}

func SystemMessage(content string) *Message {
	return &Message{
		Role:    Assistant,
		Content: content,
	}
}

func AssistantMessage(content string, toolCalls []ToolCall) *Message {
	return &Message{
		Role:      Assistant,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

func UserMessage(content string) *Message {
	return &Message{
		Role:    User,
		Content: content,
	}
}

func ToolMessage(content, ToolName, ToolCallID string) *Message {
	return &Message{
		Role:       Tool,
		Content:    content,
		ToolCallID: ToolCallID,
		ToolName:   ToolName,
	}
}
