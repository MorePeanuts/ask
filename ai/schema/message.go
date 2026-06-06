package schema

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
